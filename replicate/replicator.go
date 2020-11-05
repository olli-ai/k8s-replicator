package replicate

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// ReplicatorActions is the interface to implement for each resource type
type ReplicatorActions interface {
	// Returns the meta of a resource
	// Probably nothing more than `&object.(*ResourceType).ObjectMeta`
	GetMeta(object interface{}) *metav1.ObjectMeta
	// Updates a resource with the data from the source, and the given annotations
	Update(client kubernetes.Interface, object interface{}, sourceObject interface{}, annotations map[string]string) (interface{}, error)
	// Clears a resource from any data, and set the given annotations
	Clear(client kubernetes.Interface, object interface{}, annotations map[string]string) (interface{}, error)
	// Creates or updates the given resource with info from the source, data from the data object, and the given meta
	// create if `object.ResourceVersion == ""`, update else
	Install(client kubernetes.Interface, meta *metav1.ObjectMeta, sourceObject interface{}, dataObject interface{}) (interface{}, error)
	// Deletes the given resource
	Delete(client kubernetes.Interface, meta interface{}) (error)
}

// ObjectReplicator is the structure for any replicator
type ObjectReplicator struct {
	ReplicatorProps
	ReplicatorActions
}

// Synced returns if synched with kubernetes
func (r *ObjectReplicator) Synced() bool {
	return r.namespaceController.HasSynced() && r.objectController.HasSynced()
}

// Start starts the replicator
func (r *ObjectReplicator) Start() {
	log.Printf("running %s object controller", r.Name)
	go r.namespaceController.Run(wait.NeverStop)
	go r.objectController.Run(wait.NeverStop)
}

// InitStores inits namespace store and object store
func (r *ObjectReplicator) InitStores(lw cache.ListerWatcher, objType runtime.Object, resyncPeriod time.Duration) {
	namespaces := r.client.CoreV1().Namespaces()
	r.namespaceStore, r.namespaceController = newFilledInformer(
		&cache.ListWatch{
			ListFunc: func(lo metav1.ListOptions) (runtime.Object, error) {
				return namespaces.List(lo)
			},
			WatchFunc: namespaces.Watch,
		},
		&v1.Namespace{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: r.NamespaceAdded,
		},
	)
	r.objectStore, r.objectController = newFilledInformer(
		lw,
		objType,
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    r.ObjectAdded,
			UpdateFunc: func(old interface{}, new interface{}) {
				r.ObjectAdded(new)
			},
			DeleteFunc: r.ObjectDeleted,
		},
	)
}

// an informer that fills the store on list call
func newFilledInformer(lw cache.ListerWatcher, objType runtime.Object, resyncPeriod time.Duration, handlers cache.ResourceEventHandler) (cache.Store, cache.Controller) {
	var store cache.Store
	var controller cache.Controller
	var toAdd map[string]bool
	store, controller = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(lo metav1.ListOptions) (runtime.Object, error) {
				if object, err := lw.List(lo); err != nil {
					return object, err
				} else if list, err := meta.ListAccessor(object); err != nil {
					return object, err
				} else if items, err := meta.ExtractList(object); err != nil {
					return object, err
				// fill up the store already, to avoid thinking other resources don't exist
				} else {
					copy := make([]interface{}, len(items))
					toAdd = make(map[string]bool, len(items))
					for index, item := range items {
						copy[index] = item
						// save which one should be added at next update
						accessor, err := meta.Accessor(item)
						if err != nil {
							return object, err
						}
						toAdd[fmt.Sprintf("%s/%s", accessor.GetNamespace(), accessor.GetName())] = true
					}
					err = store.Replace(copy, list.GetResourceVersion())
					return object, err
				}
			},
			WatchFunc: lw.Watch,
		},
		objType,
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    handlers.OnAdd,
			UpdateFunc: func(old interface{}, new interface{}) {
				// because of the store fill up, an "update" event is sent instead of an "add" event
				if accessor, err := meta.Accessor(old); err == nil {
					key := fmt.Sprintf("%s/%s", accessor.GetNamespace(), accessor.GetName())
					if toAdd[key] {
						delete(toAdd, key)
						handlers.OnAdd(new)
						return
					}
				}
				handlers.OnUpdate(old, new)
			},
			DeleteFunc: handlers.OnDelete,
		},
	)
	return store, controller
}

// NamespaceAdded is called when a namespace is seen in kubernetes
// Creates the resouces that should be replicated in that namespace
func (r *ObjectReplicator) NamespaceAdded(object interface{}) {
	namespace := object.(*v1.Namespace)
	log.Printf("new namespace %s for %s replication", namespace.Name, r.Name)
	// find all the objects which want to replicate to that namespace
	todo := map[string]bool{}

	for source, watched := range r.watchedTargets {
		for _, ns := range watched {
			if namespace.Name == strings.SplitN(ns, "/", 2)[0] {
				todo[source] = true
				break
			}
		}
	}

	for source, patterns := range r.watchedPatterns {
		if todo[source] {
			continue
		}

		for _, p := range patterns {
			if p.MatchNamespace(namespace.Name) != "" {
				todo[source] = true
				break
			}
		}
	}
	// get all sources and let them replicate
	for source := range todo {
		if sourceObject, _, exists, err := r.getFromStore(source); err != nil {
			log.Printf("could not get %s %s: %s", r.Name, source, err)
		// it should not happen, but maybe `ObjectDeleted` hasn't been called yet
		// just clean watched targets to avoid this to happen again
		} else if !exists {
			log.Printf("%s %s not found", r.Name, source)
			delete(r.watchedTargets, source)
			delete(r.watchedPatterns, source)
		// let the source replicate
		} else {
			log.Printf("%s %s is watching namespace %s", r.Name, source, namespace.Name)
			r.replicateToNamespace(sourceObject, namespace.Name)
		}
	}
}

// Replicates a source to a namespace, using the replicate-to annotations
func (r *ObjectReplicator) replicateToNamespace(object interface{}, namespace string) {
	meta := r.GetMeta(object)
	key := fmt.Sprintf("%s/%s", meta.Namespace, meta.Name)
	// those annotations have priority
	if _, ok := meta.Annotations[ReplicatedByAnnotation]; ok {
		return
	}
	// get all targets
	targets, targetPatterns, err := r.getReplicationTargets(meta)
	if err != nil {
		log.Printf("could not parse %s %s: %s", r.Name, key, err)
		return
	}
	// find the ones matching with the namespace
	existingTargets := map[string]bool{}

	for _, target := range targets {
		if namespace == strings.SplitN(target, "/", 2)[0] {
			existingTargets[target] = true
		}
	}

	for _, pattern := range targetPatterns {
		if target := pattern.MatchNamespace(namespace); target != "" {
			existingTargets[target] = true
		}
	}
	// cannot target itself
	delete(existingTargets, key)
	if len(existingTargets) == 0 {
		return
	}
	// get the current targets in order to update the slice
	currentTargets, ok := r.targetsTo[key]
	if !ok {
		currentTargets = []string{}
	}
	// install all the new targets
	for target := range existingTargets {
		log.Printf("%s %s is replicated to %s", r.Name, key, target)
		currentTargets = append(currentTargets, target)
		r.installObject(target, nil, object)
	}
	// update the current targets
	r.targetsTo[key] = currentTargets
	// no need to update watched namespaces nor pattern namespaces
	// because if we are here, it means they already match this namespace
}

// ObjectAdded is called when a new resource is seen in kubernetes
// Checks its replication status and does the necessaey updates
func (r *ObjectReplicator) ObjectAdded(object interface{}) {
	meta := r.GetMeta(object)
	key := fmt.Sprintf("%s/%s", meta.Namespace, meta.Name)
	// look for unknown annotations
	if unknown := UnknownAnnotations(meta.Annotations); len(unknown) > 0 {
		for _, annotation := range unknown {
			log.Printf("unknown annotation %s on %s %s", annotation, r.Name, key)
		}
		if !r.IgnoreUnknown {
			log.Printf("could not parse %s %s: unknown annotation %s", r.Name, key, unknown[0])
			return
		}
	}
	// get replication targets
	targets, targetPatterns, err := r.getReplicationTargets(meta)
	if err != nil {
		log.Printf("could not parse %s %s: %s", r.Name, key, err)
		return
	}
	// if it was already replicated to some targets
	// check that the annotations still permit it
	if oldTargets, ok := r.targetsTo[key]; ok {
		log.Printf("source %s %s changed", r.Name, key)

		sort.Strings(oldTargets)
		previous := ""
Targets:
		for _, target := range oldTargets {
			if target == previous {
				continue Targets
			}
			previous = target

			for _, t := range targets {
				if t == target {
					continue Targets
				}
			}
			for _, p := range targetPatterns {
				if p.MatchString(target) {
					continue Targets
				}
			}
			// apparently this target is not valid anymore
			log.Printf("annotation of source %s %s changed: deleting target %s",
				r.Name, key, target)
			r.deleteObject(target, object)
		}
	}
	// clean all thos fields, they will be refilled further anyway
	delete(r.targetsTo, key)
	delete(r.watchedTargets, key)
	delete(r.watchedPatterns, key)
	// check for object having dependencies, and update them
	if replicas, ok := r.targetsFrom[key]; ok {
		log.Printf("%s %s has %d dependents", r.Name, key, len(replicas))
		r.updateDependents(object, replicas)
	}
	// this object was replicated by another, update it
	if val, ok := meta.Annotations[ReplicatedByAnnotation]; ok {
		log.Printf("%s %s is replicated by %s", r.Name, key, val)
		sourceObject, sourceMeta, exists, err := r.getFromStore(val)

		if err != nil {
			log.Printf("could not get %s %s: %s", r.Name, val, err)
			return
		// the source has been deleted, so should this object be
		} else if !exists {
			log.Printf("source %s %s deleted: deleting target %s", r.Name, val, key)

		} else if ok, err := r.isReplicatedTo(sourceMeta, meta); err != nil {
			log.Printf("could not parse %s %s: %s", r.Name, val, err)
			return
		// the source annotations have changed, this replication is deleted
		} else if !ok {
			log.Printf("source %s %s is not replicated to %s: deleting target", r.Name, val, key)
			exists = false
		}
		// no source, delete it
		if !exists {
			r.doDeleteObject(object)
			return
		// source is here, install it
		} else if err := r.installObject("", object, sourceObject); err != nil {
			return
		// get it back after edit
		} else if obj, m, err := r.requireFromStore(key); err != nil {
			log.Printf("could not get %s %s: %s", r.Name, key, err)
			return
		// continue
		} else {
			object = obj
			meta = m
			targets = nil
			targetPatterns = nil
		}
	}
	// this object is replicated to other locations
	if targets != nil || targetPatterns != nil {
		existsNamespaces := map[string]bool{} // a cache to remember the done lookups
		existingTargets := []string{} // the slice of all the target this object should replicate to

		for _, t := range targets {
			ns := strings.SplitN(t, "/", 2)[0]
			var exists, ok bool
			var err error
			// already in cache
			if exists, ok = existsNamespaces[ns]; ok {
			// get it
			} else if _, exists, err = r.namespaceStore.GetByKey(ns); err == nil {
				existsNamespaces[ns] = exists
			}

			if err != nil {
				log.Printf("could not get namespace %s: %s", ns, err)
			} else if exists {
				existingTargets = append(existingTargets, t)
			} else {
				log.Printf("replication of %s %s to %s cancelled: no namespace %s",
					r.Name, key, t, ns)
			}
		}

		if len(targetPatterns) > 0 {
			namespaces := r.namespaceStore.ListKeys()
			// cache all existing targets
			seen := map[string]bool{key: true}
			for _, t := range existingTargets {
				seen[t] = true
			}
			// find which new targets match the patterns
			for _, p := range targetPatterns {
				for _, t := range p.Targets(namespaces) {
					if !seen[t] {
						seen[t] = true
						existingTargets = append(existingTargets, t)
					}
				}
			}
		}
		// save all those info
		if len(targets) > 0 {
			r.watchedTargets[key] = targets
		}

		if len(targetPatterns) > 0 {
			r.watchedPatterns[key] = targetPatterns
		}

		if len(existingTargets) > 0 {
			r.targetsTo[key] = existingTargets
			// create all targets
			for _, t := range existingTargets {
				log.Printf("%s %s is replicated to %s", r.Name, key, t)
				r.installObject(t, nil, object)
			}
		}
		// in this case, replicate-from annoation only refers to the target
		// so should stop now
		return
	}
	// this object is replicated from another, update it
	if val, ok := resolveAnnotation(meta, ReplicateFromAnnotation); ok {
		log.Printf("%s %s is replicated from %s", r.Name, key, val)
		// update the dependencies of the source, even if it maybe does not exist yet
		if _, ok := r.targetsFrom[val]; !ok {
			r.targetsFrom[val] = make([]string, 0, 1)
		}
		r.targetsFrom[val] = append(r.targetsFrom[val], key)

		if sourceObject, _, exists, err := r.getFromStore(val); err != nil {
			log.Printf("could not get %s %s: %s", r.Name, val, err)
			return
		// the source does not exist anymore/yet, clear the data of the target
		} else if !exists {
			log.Printf("source %s %s deleted: clearing target %s", r.Name, val, key)
			r.doClearObject(object)
		// update the target
		} else {
			r.replicateObject(object, sourceObject)
		}
	}
}

// Replicates a resource that has a replicate-from annotation from its source
func (r *ObjectReplicator) replicateObject(object interface{}, sourceObject  interface{}) error {
	meta := r.GetMeta(object)
	sourceMeta := r.GetMeta(sourceObject)
	// make sure replication is allowed
	if ok, nok, err := r.isReplicationAllowed(meta, sourceMeta); ok {
	} else if nok {
		log.Printf("replication of %s %s/%s is not allowed: %s", r.Name, meta.Namespace, meta.Name, err)
		return r.doClearObject(object)
	} else {
		log.Printf("replication of %s %s/%s is cancelled: %s", r.Name, meta.Namespace, meta.Name, err)
		return err
	}
	// check if replication is needed
	if ok, _, err := r.needsDataUpdate(meta, sourceMeta); !ok {
		log.Printf("replication of %s %s/%s is skipped: %s", r.Name, meta.Namespace, meta.Name, err)
		return err
	}
	// build the annotations
	annotations := map[string]string{}
	for key, val := range meta.Annotations {
		annotations[key] = val
	}
	annotations[ReplicatedAtAnnotation] = time.Now().Format(time.RFC3339)
	annotations[ReplicatedFromVersionAnnotation] = sourceMeta.ResourceVersion
	if val, ok := sourceMeta.Annotations[ReplicateOnceVersionAnnotation]; ok {
		annotations[ReplicateOnceVersionAnnotation] = val
	} else {
		delete(annotations, ReplicateOnceVersionAnnotation)
	}
	// replicate it
	newObject, err := r.Update(r.client, object, sourceObject, annotations)
	// update the object store in advance, to avoid being disturbed later
	if err == nil {
		err = r.objectStore.Update(newObject)
	}
	return err
}

// Repliates a resource that has a replicate-to annotation to its target
// Pass either target string or targetObject object
func (r *ObjectReplicator) installObject(target string, targetObject interface{}, sourceObject interface{}) error {
	var targetMeta *metav1.ObjectMeta
	sourceMeta := r.GetMeta(sourceObject)
	var targetSplit []string // similar to target, but splitted in 2
	// targetObject was not passed, check if it exists
	if targetObject == nil {
		targetSplit = strings.SplitN(target, "/", 2)
		// invalid target
		if len(targetSplit) != 2 {
			err := fmt.Errorf("illformed annotation %s in %s %s/%s: expected namespace/name, got %s",
				ReplicatedByAnnotation, r.Name, sourceMeta.Namespace, sourceMeta.Name, target)
			log.Printf("%s", err)
			return err
		}

		var exists bool
		var err error
		// error while getting the target
		if targetObject, targetMeta, exists, err = r.getFromStore(target); err != nil {
			log.Printf("could not get %s %s: %s", r.Name, target, err)
			return err
		// the target exists already
		} else if exists {
			// check if target was created by replication from source
			if ok, err := r.isReplicatedBy(targetMeta, sourceMeta); !ok {
				log.Printf("replication of %s %s/%s is cancelled: %s",
					r.Name, sourceMeta.Namespace, sourceMeta.Name, err)
				return err
			}
		}
	// targetObject was passed already
	} else {
		targetMeta = r.GetMeta(targetObject)
		targetSplit = []string{targetMeta.Namespace, targetMeta.Name}
	}
	// the data must come from another object
	if source, ok := resolveAnnotation(sourceMeta, ReplicateFromAnnotation); ok {
		if targetMeta != nil {
			// Check if needs an annotations update
			if ok, err := r.needsFromAnnotationsUpdate(targetMeta, sourceMeta); err != nil {
				log.Printf("replication of %s %s/%s is cancelled: %s",
					r.Name, sourceMeta.Namespace, sourceMeta.Name, err)
				return err

			} else if !ok {
				return nil
			}
		}
		// create a new meta with all the annotations
		copyMeta := metav1.ObjectMeta{
			Namespace:   targetSplit[0],
			Name:        targetSplit[1],
			Labels:      make(map[string]string, len(r.Labels)),
			Annotations: make(map[string]string, 3),
		}
		// copy the labels
		for key, value := range r.Labels {
			copyMeta.Labels[key] = value
		}
		// add the annotations
		copyMeta.Annotations[ReplicatedByAnnotation] = fmt.Sprintf("%s/%s",
			sourceMeta.Namespace, sourceMeta.Name)
		copyMeta.Annotations[ReplicateFromAnnotation] = source
		if val, ok := sourceMeta.Annotations[ReplicateOnceAnnotation]; ok {
			copyMeta.Annotations[ReplicateOnceAnnotation] = val
		}
		// Needs ResourceVersion for update
		if targetMeta != nil {
			copyMeta.ResourceVersion = targetMeta.ResourceVersion
		}

		log.Printf("installing %s %s/%s: updating replicate-from annotations", r.Name, copyMeta.Namespace, copyMeta.Name)
		// install it, but keeps the original data
		newObject, err := r.Install(r.client, &copyMeta, sourceObject, targetObject)
		// update the object store in advance, to avoid being disturbed later
		if err == nil {
			err = r.objectStore.Update(newObject)
		}
		return err
	}
	// the data comes directly from the source
	if targetMeta != nil {
		// the target was previously replicated from another source
		// replication is required
		if _, ok := targetMeta.Annotations[ReplicateFromAnnotation]; ok {
		// checks that the target is up to date
		} else if ok, once, err := r.needsDataUpdate(targetMeta, sourceMeta); !ok {
			// check that the target needs replication-allowed annotations update
			if (!once) {
			} else if ok, err2 := r.needsAllowedAnnotationsUpdate(targetMeta, sourceMeta); err2 != nil {
				err = err2
			} else if ok {
				err = nil
			}
			if (err != nil) {
				log.Printf("replication of %s %s/%s is skipped: %s",
					r.Name, sourceMeta.Namespace, sourceMeta.Name, err)
				return err
			}
			// copy the target but update replication-allowed annotations
			copyMeta := targetMeta.DeepCopy()
			if val, ok := sourceMeta.Annotations[ReplicationAllowedAnnotation]; ok {
				copyMeta.Annotations[ReplicationAllowedAnnotation] = val
			} else {
				delete(copyMeta.Annotations, ReplicationAllowedAnnotation)
			}
			if val, ok := sourceMeta.Annotations[ReplicationAllowedNsAnnotation]; ok {
				copyMeta.Annotations[ReplicationAllowedNsAnnotation] = val
			} else {
				delete(copyMeta.Annotations, ReplicationAllowedNsAnnotation)
			}

			log.Printf("installing %s %s/%s: updating replication-allowed annotations", r.Name, copyMeta.Namespace, copyMeta.Name)
			// install it with the original data
			newObject, err := r.Install(r.client, copyMeta, sourceObject, targetObject)
			// update the object store in advance, to avoid being disturbed later
			if err == nil {
				err = r.objectStore.Update(newObject)
			}
			return err
		}
	}
	// create a new meta with all the annotations
	copyMeta := metav1.ObjectMeta{
		Namespace:   targetSplit[0],
		Name:        targetSplit[1],
		Labels:      make(map[string]string, len(r.Labels)),
		Annotations: make(map[string]string, 6),
	}
	// copy the labels
	for key, value := range r.Labels {
		copyMeta.Labels[key] = value
	}
	// add the annotations
	copyMeta.Annotations[ReplicatedAtAnnotation] = time.Now().Format(time.RFC3339)
	copyMeta.Annotations[ReplicatedByAnnotation] = fmt.Sprintf("%s/%s",
		sourceMeta.Namespace, sourceMeta.Name)
	copyMeta.Annotations[ReplicatedFromVersionAnnotation] = sourceMeta.ResourceVersion
	if val, ok := sourceMeta.Annotations[ReplicateOnceVersionAnnotation]; ok {
		copyMeta.Annotations[ReplicateOnceVersionAnnotation] = val
	}
	// replicate authorization annotations too
	if val, ok := sourceMeta.Annotations[ReplicationAllowedAnnotation]; ok {
		copyMeta.Annotations[ReplicationAllowedAnnotation] = val
	}
	if val, ok := sourceMeta.Annotations[ReplicationAllowedNsAnnotation]; ok {
		copyMeta.Annotations[ReplicationAllowedNsAnnotation] = val
	}
	// Needs ResourceVersion for update
	if targetMeta != nil {
		copyMeta.ResourceVersion = targetMeta.ResourceVersion
	}

	log.Printf("installing %s %s/%s: updating data", r.Name, copyMeta.Namespace, copyMeta.Name)
	// install it with the source data
	newObject, err := r.Install(r.client, &copyMeta, sourceObject, sourceObject)
	// update the object store in advance, to avoid being disturbed later
	if err == nil {
		err = r.objectStore.Update(newObject)
	}
	return err
}

// Gets a resource from the object store
// Returns:
//  - object: the resource, if present in the object store
//  - meta: the resource's meta, if present in the object store
//  - present: if present
//  - err: on error
func (r *ObjectReplicator) getFromStore(key string) (interface{}, *metav1.ObjectMeta, bool, error) {
	object, exists, err := r.objectStore.GetByKey(key)
	if err != nil || !exists {
		return nil, nil, exists, err
	}
	meta := r.GetMeta(object)
	if !r.IgnoreUnknown {
		unknown := UnknownAnnotations(r.GetMeta(object).Annotations)
		for _, annotation := range unknown {
			log.Printf("unknown annotation %s on %s %s", annotation, r.Name, key)
		}
		if len(unknown) > 0 {
			return nil, nil, false, fmt.Errorf("unknown annotation %s", unknown[0])
		}
	}
	return object, meta, true, nil
}

// Gets a resource from the object store, returns an error if not present
// Returns:
//  - object: the resource, if present in the object store
//  - meta: the resource's meta, if present in the object store
//  - err: on error or if not present
func (r *ObjectReplicator) requireFromStore(key string) (interface{}, *metav1.ObjectMeta, error) {
	object, meta, exists, err := r.getFromStore(key)
	if err == nil && !exists {
		return nil, nil, fmt.Errorf("does not exist")
	}
	return object, meta, err
}

// Updates the list of all target resources that should be notified when the source is updated
func (r *ObjectReplicator) updateDependents(object interface{}, replicas []string) error {
	meta := r.GetMeta(object)
	key := fmt.Sprintf("%s/%s", meta.Namespace, meta.Name)

	sort.Strings(replicas)
	updatedReplicas := make([]string, 0, 0)
	var previous string

	for _, dependentKey := range replicas {
		// get rid of dupplicates in replicas
		if previous == dependentKey {
			continue
		}
		previous = dependentKey

		targetObject, targetMeta, err := r.requireFromStore(dependentKey)
		if err != nil {
			log.Printf("could not load dependent %s %s: %s", r.Name, dependentKey, err)
			continue
		}

		if val, ok := resolveAnnotation(targetMeta, ReplicateFromAnnotation); !ok || val != key {
			log.Printf("annotation of dependent %s %s changed", r.Name, dependentKey)
			continue
		}

		updatedReplicas = append(updatedReplicas, dependentKey)

		r.replicateObject(targetObject, object)
	}

	if len(updatedReplicas) > 0 {
		r.targetsFrom[key] = updatedReplicas
	} else {
		delete(r.targetsFrom, key)
	}

	return nil
}

// ObjectDeleted is called when a resource is updated
// Checks if a target should be cleared / deleted, or if it should be replaced by a replication
func (r *ObjectReplicator) ObjectDeleted(object interface{}) {
	meta := r.GetMeta(object)
	key := fmt.Sprintf("%s/%s", meta.Namespace, meta.Name)
	// delete targets of replicate-to annotations
	if targets, ok := r.targetsTo[key]; ok {
		for _, t := range targets {
			r.deleteObject(t, object)
		}
	}
	delete(r.targetsTo, key)
	delete(r.watchedTargets, key)
	delete(r.watchedPatterns, key)
	// clear targets of replicate-from annotations
	if replicas, ok := r.targetsFrom[key]; ok {
		sort.Strings(replicas)
		updatedReplicas := make([]string, 0, 0)
		var previous string

		for _, dependentKey := range replicas {
			// get rid of dupplicates in replicas
			if previous == dependentKey {
				continue
			}
			previous = dependentKey

			if ok, _ := r.clearObject(dependentKey, object); ok {
				updatedReplicas = append(updatedReplicas, dependentKey)
			}
		}

		if len(updatedReplicas) > 0 {
			r.targetsFrom[key] = updatedReplicas
		} else {
			delete(r.targetsFrom, key)
		}
	}
	// find which source want to replicate into this object, now that they can
	todo := map[string]bool{}

	for source, watched := range r.watchedTargets {
		for _, t := range watched {
			if key == t {
				todo[source] = true
				break
			}
		}
	}

	for source, patterns := range r.watchedPatterns {
		if todo[source] {
			continue
		}

		for _, p := range patterns {
			if p.Match(meta) {
				todo[source] = true
				break
			}
		}
	}
	// find the first source that still wants to replicate
	for source := range todo {
		if sourceObject, sourceMeta, exists, err := r.getFromStore(source); err != nil {
			log.Printf("could not get %s %s: %s", r.Name, source, err)
		// it should not happen, but maybe `ObjectDeleted` hasn't been called yet
		// just clean watched targets to avoid this to happen again
		} else if !exists {
			log.Printf("%s %s not found", r.Name, source)
			delete(r.watchedTargets, source)
			delete(r.watchedPatterns, source)

		} else if ok, err := r.isReplicatedTo(sourceMeta, meta); err != nil {
			log.Printf("could not parse %s %s: %s", r.Name, source, err)
		// the source sitll want to be replicated, so let's do it
		} else if ok {
			r.installObject(key, nil, sourceObject)
			break
		}
	}
}

// Clear a resource's data, because its source has been deleted or doesn't allow replication anymore
func (r *ObjectReplicator) clearObject(key string, sourceObject interface{}) (bool, error) {
	sourceMeta := r.GetMeta(sourceObject)

	targetObject, targetMeta, err := r.requireFromStore(key)
	if err != nil {
		log.Printf("could not load dependent %s %s: %s", r.Name, key, err)
		return false, err
	}

	if !annotationRefersTo(targetMeta, ReplicateFromAnnotation, sourceMeta) {
		log.Printf("annotation of dependent %s %s changed", r.Name, key)
		return false, nil
	}

	return true, r.doClearObject(targetObject)
}

// Actually clear the object, no further check needed
func (r *ObjectReplicator) doClearObject(object interface{}) error {
	meta := r.GetMeta(object)

	if _, ok := meta.Annotations[ReplicatedFromVersionAnnotation]; !ok {
		log.Printf("%s %s/%s is already up-to-date", r.Name, meta.Namespace, meta.Name)
		return nil
	}
	// build the annotations
	annotations := map[string]string{}
	for key, val := range meta.Annotations {
		annotations[key] = val
	}
	annotations[ReplicatedAtAnnotation] = time.Now().Format(time.RFC3339)
	delete(annotations, ReplicatedFromVersionAnnotation)
	delete(annotations, ReplicateOnceVersionAnnotation)

	newObject, err := r.Clear(r.client, object, annotations)
	// update the object store in advance, to avoid being disturbed later
	if err == nil {
		err = r.objectStore.Update(newObject)
	}
	return err
}

// Deletes a resource, because its source was deleted or stopped replication
func (r *ObjectReplicator) deleteObject(key string, sourceObject interface{}) (bool, error) {
	sourceMeta := r.GetMeta(sourceObject)

	object, meta, err := r.requireFromStore(key)
	if err != nil {
		log.Printf("could not get %s %s: %s", r.Name, key, err)
		return false, err
	}

	// make sure replication is allowed
	if ok, err := r.isReplicatedBy(meta, sourceMeta); !ok {
		log.Printf("deletion of %s %s is cancelled: %s", r.Name, key, err)
		return false, err
	}
	// delete the object
	return true, r.doDeleteObject(object)
}

// Actually delete the object, no further check needed
func (r *ObjectReplicator) doDeleteObject(object interface{}) error {
	err := r.Delete(r.client, object)
	// update the object store in advance, to avoid being disturbed later
	if err == nil {
		err = r.objectStore.Delete(object)
	}
	return err
}
