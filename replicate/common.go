// Various non specific functions to manipulate objects

package replicate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// pattern of a valid kubernetes name
var validName = regexp.MustCompile(`^[0-9a-z.-]+$`)
var validPath = regexp.MustCompile(`^(?:[0-9a-z.-]+/)?[0-9a-z.-]+$`)

// a struct representing a pattern to match namespaces and generating targets
type targetPattern struct {
	namespace *regexp.Regexp
	name      string
}
// Returns true if the pattern matches the given target object
func (pattern targetPattern) Match(object *metav1.ObjectMeta) bool {
	return object.Name == pattern.name && pattern.namespace.MatchString(object.Namespace)
}
// Returns true if the pattern matches the given target path
func (pattern targetPattern) MatchString(target string) bool {
	parts := strings.SplitN(target, "/", 2)
	return len(parts) == 2 && parts[1] == pattern.name && pattern.namespace.MatchString(parts[0])
}
// Returns a target path in this namespace if the namespace is matching
func (pattern targetPattern) MatchNamespace(namespace string) string {
	if pattern.namespace.MatchString(namespace) {
		return fmt.Sprintf("%s/%s", namespace, pattern.name)
	}
	return ""
}
// Returns a slice of targets paths in the matching namespaces
func (pattern targetPattern) Targets(namespaces []string) []string {
	suffix := "/" + pattern.name
	targets := []string{}
	for _, ns := range namespaces {
		if pattern.namespace.MatchString(ns) {
			targets = append(targets, ns+suffix)
		}
	}
	return targets
}

type ReplicatorOptions struct {
	// when true, "allowed" annotations are ignored
	AllowAll      bool
	// when false, any unknown annotation will make the replicator fail
	IgnoreUnknown bool
}

type ReplicatorProps struct {
	// displayed name for the resources
	Name                string
	// various options
	ReplicatorOptions
	// the kubernetes client to use
	client              kubernetes.Interface

	// the store and controller for all the objects to watch replicate
	objectStore         cache.Store
	objectController    cache.Controller

	// the store and controller for the namespaces
	namespaceStore      cache.Store
	namespaceController cache.Controller

	// a {source => targets} map for the "replicate-from" annotation
	targetsFrom         map[string][]string
	// a {source => targets} map for the "replicate-to" annotation
	targetsTo           map[string][]string

	// a {source => targets} map for all the targeted objects
	watchedTargets      map[string][]string
	// a {source => targetPatterns} for all the targeted objects
	watchedPatterns     map[string][]targetPattern
}

// Replicator describes the common interface that the secret and configmap
// replicators should adhere to
type Replicator interface {
	Start()
	Synced() bool
}

func NewReplicatorProps(client kubernetes.Interface, name string, options ReplicatorOptions) ReplicatorProps {
	return ReplicatorProps {
		Name:                name,
		ReplicatorOptions:   options,
		client:              client,

		targetsFrom:         map[string][]string{},
		targetsTo:           map[string][]string{},

		watchedTargets:      map[string][]string{},
		watchedPatterns:     map[string][]targetPattern{},
	}
}

// Checks if replication is allowed in annotations of the source object.
// This is checked anytime a target object tries to replicate a source object using the replicate-from annotation
// Replication is allowed if all those conditions are met:
//	- replication-allowed and replication-allowed-namespaces annotations are valid
//	- the annotations explictely allow replication when present
//	- the annoations are present, or --allow-all parameter is set
// Returns:
//	- allowed: true if replication is allowed.
//  - disallowed: true if replication is explicitely or implicitely disallowed
//	- err: if the replication is not allowed, an error message
func (r *ReplicatorProps) isReplicationAllowed(object *metav1.ObjectMeta, sourceObject *metav1.ObjectMeta) (bool, bool, error) {
	// read the annotations
	annotationAllowed, ok := sourceObject.Annotations[ReplicationAllowedAnnotation]
	annotationAllowedNs, okNs := sourceObject.Annotations[ReplicationAllowedNsAnnotation]
	// unless AllowAll, explicit permission is required
	if !r.AllowAll && !ok && !okNs {
		return false, true, fmt.Errorf("source %s/%s does not explicitely allow replication",
			sourceObject.Namespace, sourceObject.Name)
	}
	// check allow annotation
	if ok {
		// the annotation is not a boolean
		if val, err := strconv.ParseBool(annotationAllowed); err != nil {
			return false, false, fmt.Errorf("source %s/%s has illformed annotation %s (%s): %s",
				sourceObject.Namespace, sourceObject.Name, ReplicationAllowedAnnotation, annotationAllowed, err)
		// the annotations is "false"
		} else if !val {
			return false, true, fmt.Errorf("source %s/%s explicitely disallow replication",
				sourceObject.Namespace, sourceObject.Name)
		}
	}
	// check allow-namespaces annotation
	if okNs {
		allowed := false
		for _, ns := range strings.Split(annotationAllowedNs, ",") {
			if ns == "" {
			// an namespace, allowed if equal
			} else if validName.MatchString(ns) {
				if ns == object.Namespace {
					allowed = true
				}
			// a namespace pattern, allowed if matching
			} else if ok, err := regexp.MatchString(`^(?:`+ns+`)$`, object.Namespace); ok {
				allowed = true
			// the pattern is invalid
			} else if err != nil {
				return false, false, fmt.Errorf("source %s/%s has compilation error on annotation %s (%s): %s",
					sourceObject.Namespace, sourceObject.Name, ReplicationAllowedNsAnnotation, ns, err)
			}
		}
		// the namespace is not allowed
		if !allowed {
			return false, true, fmt.Errorf("source %s/%s does not allow replication to namespace %s",
				sourceObject.Namespace, sourceObject.Name, object.Namespace)
		}
	}
	// source cannot have "replicate-from" annotation
	if val, ok := resolveAnnotation(sourceObject, ReplicateFromAnnotation); ok {
		return false, true, fmt.Errorf("source %s/%s is already replicated from %s",
			sourceObject.Namespace, sourceObject.Name, val)
	}

	return true, false, nil
}

// Checks that data update is needed
// This is checked for every target object that should receive a copy of the data of the source object
// Data update is not needed in one of those cases
//	- any annotation is incorrect
//	- the target replicated-from-version annotation matches with the source resource version
//  - the source or the target has the replicate-once annotation, and the target replicate-once-version is up to date
// Returns:
//	- ok: true if an update is needed
//	- once: true if no update is needed because the object is replicated once
//	- err: an error message if no update is needed
func (r *ReplicatorProps) needsDataUpdate(object *metav1.ObjectMeta, sourceObject *metav1.ObjectMeta) (bool, bool, error) {
	// target was "replicated" from a delete source, or never replicated
	if targetVersion, ok := object.Annotations[ReplicatedFromVersionAnnotation]; !ok {
		return true, false, nil
	// target and source share the same version
	} else if targetVersion == sourceObject.ResourceVersion {
		return false, false, fmt.Errorf("target %s/%s is already up-to-date", object.Namespace, object.Name)
	}

	// check the once annotations

	hasOnce := false
	// no source once annotation, nothing to check
	if annotationOnce, ok := sourceObject.Annotations[ReplicateOnceAnnotation]; !ok {
	// source once annotation is not a boolean
	} else if once, err := strconv.ParseBool(annotationOnce); err != nil {
		return false, false, fmt.Errorf("source %s/%s has illformed annotation %s: %s",
			sourceObject.Namespace, sourceObject.Name, ReplicateOnceAnnotation, err)
	// source once annotation is present
	} else if once {
		hasOnce = true
	}
	// no target once annotation, nothing to check
	if annotationOnce, ok := object.Annotations[ReplicateOnceAnnotation]; !ok {
	// target once annotation is not a boolean
	} else if once, err := strconv.ParseBool(annotationOnce); err != nil {
		return false, false, fmt.Errorf("target %s/%s has illformed annotation %s: %s",
			object.Namespace, object.Name, ReplicateOnceAnnotation, err)
	// target once annotation is present
	} else if once {
		hasOnce = true
	}

	// check the version annotations

	if !hasOnce {
	} else if sourceVersion, ok := sourceObject.Annotations[ReplicateOnceVersionAnnotation]; !ok {
		return false, true, fmt.Errorf("target %s/%s is already replicated once",
			object.Namespace, object.Name)
	} else if version, ok := object.Annotations[ReplicateOnceVersionAnnotation]; ok && sourceVersion == version {
		return false, true, fmt.Errorf("target %s/%s is already replicated once at current version",
			object.Namespace, object.Name)
	}

	// replication is needed
	return true, false, nil
}

// Checks that replicate-from and replicate-once annotations update is needed
// This is checked when a source object defines both replicate-from and replicate-to annotation: the target object must replicate its replicate-from and replicate-once annotations
// Annotations update is not required in those cases:
//	- an annotation is invalid
//	- the target's replicate-from and replicate-once annotations are the same as the source's annotations
// Returns:
//	- ok: true if an update is needed
//	- err: an error message if an annotation is invalid
func (r *ReplicatorProps) needsFromAnnotationsUpdate(object *metav1.ObjectMeta, sourceObject *metav1.ObjectMeta) (bool, error) {
	update := false
	// check the "from" annotation
	// the source "from" annotation is missing
	if source, sOk := resolveAnnotation(sourceObject, ReplicateFromAnnotation); !sOk {
		return false, fmt.Errorf("source %s/%s misses annotation %s",
			sourceObject.Namespace, sourceObject.Name, ReplicateFromAnnotation)
	// the source "from" annotation is invalid
	} else if !validPath.MatchString(source) ||
			source == fmt.Sprintf("%s/%s", sourceObject.Namespace, sourceObject.Name) {
		return false, fmt.Errorf("source %s/%s has invalid annotation %s (%s)",
			sourceObject.Namespace, sourceObject.Name, ReplicateFromAnnotation, source)
	// the target has different "from" annotation, update
	} else if val, ok := object.Annotations[ReplicateFromAnnotation]; !ok || val != source {
		update = true
	}

	// check "once" annotation of the source
	source, sOk := sourceObject.Annotations[ReplicateOnceAnnotation]
	// the source "once" annotation is not a boolean
	if sOk {
		if _, err := strconv.ParseBool(source); err != nil {
			return false, fmt.Errorf("source %s/%s has illformed annotation %s: %s",
				sourceObject.Namespace, sourceObject.Name, ReplicateOnceAnnotation, err)
		}
	}
	// the target has different "once" annotation, update
	if val, ok := object.Annotations[ReplicateOnceAnnotation]; sOk != ok || ok && val != source {
		update = true
	}

	return update, nil
}

// Checks that replication-allowed and replication-allowed-namespaces annotations update is needed
// This is checked when a source object defines the replicate-to or replicate-to-namespaces annoations (and not the replicate-from annotation): the target must then copy the replication-allowed and replication-allowed-namespaces
// Annotations update is not required in those cases:
//	- an annotation is invalid
//	- the target's replication-allowed and replication-allowed-namespaces annotations are the same as the source's annotations
// Returns:
//	- ok: true if an update is needed
//	- err: an error message if an annotation is invalid
func (r *ReplicatorProps) needsAllowedAnnotationsUpdate(object *metav1.ObjectMeta, sourceObject *metav1.ObjectMeta) (bool, error) {
	update := false

	allowed, okA := sourceObject.Annotations[ReplicationAllowedAnnotation]
	if val, ok := object.Annotations[ReplicationAllowedAnnotation]; ok != okA || ok && val != allowed {
		update = true
	}

	allowedNs, okNs := sourceObject.Annotations[ReplicationAllowedNsAnnotation]
	if val, ok := object.Annotations[ReplicationAllowedNsAnnotation]; ok != okNs || ok && val != allowedNs {
		update = true
	}

	if !update {
		return false, nil
	}

	// check allow annotation
	if okA {
		if _, err := strconv.ParseBool(allowed); err != nil {
			return false, fmt.Errorf("source %s/%s has illformed annotation %s (%s): %s",
				sourceObject.Namespace, sourceObject.Name, ReplicationAllowedAnnotation, allowed, err)
		}
	}
	// check allow-namespaces annotation
	if okNs {
		for _, ns := range strings.Split(allowedNs, ",") {
			if ns == "" || validName.MatchString(ns) {
			} else if _, err := regexp.Compile(`^(?:`+ns+`)$`); err != nil {
				return false, fmt.Errorf("source %s/%s has compilation error on annotation %s (%s): %s",
					sourceObject.Namespace, sourceObject.Name, ReplicationAllowedNsAnnotation, ns, err)
			}
		}
	}

	return true, nil
}

// Checks that the target is a replication from the source
// It is checked before updating a replication target, using the replicated-by annotation
// Returns:
//	- ok: true if the target was replicated by the source
//	- err: an error message if it was not
func (r *ReplicatorProps) isReplicatedBy(object *metav1.ObjectMeta, sourceObject *metav1.ObjectMeta) (bool, error) {
	// make sure that the target object was created from the source
	if annotationFrom, ok := object.Annotations[ReplicatedByAnnotation]; !ok {
		return false, fmt.Errorf("target %s/%s was not replicated",
			object.Namespace, object.Name)

	} else if annotationFrom != fmt.Sprintf("%s/%s", sourceObject.Namespace, sourceObject.Name) {
		return false, fmt.Errorf("target %s/%s was not replicated from %s/%s",
			object.Namespace, object.Name, sourceObject.Namespace, sourceObject.Name)
	}

	return true, nil
}

// Checks that the source was replicated to the target
// It is checked when the target is discovered, using the replicate-to and replicate-to-namespace annoations of the source
// Returns:
//	- ok: true if the source is replicated to the target
//	- err: an error message only if the annotations were incorrect
func (r *ReplicatorProps) isReplicatedTo(object *metav1.ObjectMeta, targetObject *metav1.ObjectMeta) (bool, error) {
	targets, targetPatterns, err := r.getReplicationTargets(object)
	if err != nil {
		return false, err
	}

	key := fmt.Sprintf("%s/%s", targetObject.Namespace, targetObject.Name)
	for _, t := range targets {
		if t == key {
			return true, nil
		}
	}

	for _, p := range targetPatterns {
		if p.Match(targetObject) {
			return true, nil
		}
	}

	return false, nil

	// return false, fmt.Error("source %s/%s is not replated to %s",
	// 	object.Namespace, object.Name, key)
}

// Returns everything needed to compute the desired targets
// - targets: a slice of all fully qualified target. Items are unique, does not contain object itself
// - targetPatterns: a slice of targetPattern, using regex to identify if a namespace is matched
//                   two patterns can generate the same target, and even the object itself
func (r *ReplicatorProps) getReplicationTargets(object *metav1.ObjectMeta) ([]string, []targetPattern, error) {
	annotationTo, okTo := object.Annotations[ReplicateToAnnotation]
	annotationToNs, okToNs := object.Annotations[ReplicateToNsAnnotation]
	if !okTo && !okToNs {
		return nil, nil, nil
	}

	key := fmt.Sprintf("%s/%s", object.Namespace, object.Name)
	targets := []string{}
	targetPatterns := []targetPattern{}
	// cache of patterns, to reuse them as much as possible
	compiledPatterns := map[string]*regexp.Regexp{}
	for _, pattern := range r.watchedPatterns[key] {
		compiledPatterns[pattern.namespace.String()] = pattern.namespace
	}
	// function to compile the namespace pattern after a cache lookup
	compileNamespace := func (ns string) (*regexp.Regexp, error) {
		pattern := `^(?:`+ns+`)$`
		// look in the pattern cache
		if p, ok := compiledPatterns[pattern]; ok {
			return p, nil
		// compile the pattern
		} else if p, err := regexp.Compile(pattern); err == nil {
			compiledPatterns[pattern] = p
			return p, nil
		// raise compilation error
		} else {
			return nil, err
		}
	}
	// which qualified paths have already been seen (exclude the object itself)
	seen := map[string]bool{key: true}
	var names, namespaces, qualified map[string]bool
	// no target explecitely provided, assumed that targets will have the same name
	if !okTo {
		names = map[string]bool{object.Name: true}
	// split the targets, and check which one are qualified
	} else {
		names = map[string]bool{}
		qualified = map[string]bool{}
		for _, n := range strings.Split(annotationTo, ",") {
			if n == "" {
			// a qualified name, with a namespace part
			} else if strings.ContainsAny(n, "/") {
				qualified[n] = true
			// a valid name
			} else if validName.MatchString(n) {
				names[n] = true
			// raise error
			} else {
				return nil, nil, fmt.Errorf("source %s has invalid name on annotation %s (%s)",
					key, ReplicateToAnnotation, n)
			}
		}
	}
	// no target namespace provided, assume that the namespace is the same (or qualified in the name)
	if !okToNs {
		namespaces = map[string]bool{object.Namespace: true}
	// split the target namespaces
	} else {
		namespaces = map[string]bool{}
		for _, ns := range strings.Split(annotationToNs, ",") {
			if strings.ContainsAny(ns, "/") {
				return nil, nil, fmt.Errorf("source %s has invalid namespace pattern on annotation %s (%s)",
					key, ReplicateToNsAnnotation, ns)
			} else if ns != "" {
				namespaces[ns] = true
			}
		}
	}
	// join all the namespaces and names
	for ns := range namespaces {
		// this namespace is not a pattern, append it in targets
		if validName.MatchString(ns) {
			ns = ns + "/"
			for n := range names {
				full := ns + n
				if !seen[full] {
					seen[full] = true
					targets = append(targets, full)
				}
			}
			continue
		// this namespace is a pattern, append it in targetPatterns
		} else if pattern, err := compileNamespace(ns); err == nil {
			ns = ns + "/"
			for n := range names {
				full := ns + n
				if !seen[full] {
					seen[full] = true
					targetPatterns = append(targetPatterns, targetPattern{pattern, n})
				}
			}
		// raise compilation error
		} else {
			return nil, nil, fmt.Errorf("source %s has compilation error on annotation %s (%s): %s",
				key, ReplicateToNsAnnotation, ns, err)
		}
	}
	// for all the qualified names, check if the namespace part is a pattern
	for q := range qualified {
		if seen[q] {
		// check that there is exactly one "/"
		} else if qs := strings.SplitN(q, "/", 3); len(qs) != 2 {
			return nil, nil, fmt.Errorf("source %s has invalid path on annotation %s (%s)",
				key, ReplicateToAnnotation, q)
		// check that the name part is valid
		} else if n := qs[1]; !validName.MatchString(n) {
			return nil, nil, fmt.Errorf("source %s has invalid name on annotation %s (%s)",
				key, ReplicateToAnnotation, n)
		// the namespace is not a pattern, append it in targets
		} else if ns := qs[0]; validName.MatchString(ns) {
			targets = append(targets, q)
		// the namespace is a pattern, append it in targetPatterns
		} else if pattern, err := compileNamespace(ns); err == nil {
			targetPatterns = append(targetPatterns, targetPattern{pattern, n})
		// raise compilation error
		} else {
			return nil, nil, fmt.Errorf("source %s has compilation error on annotation %s (%s): %s",
				key, ReplicateToAnnotation, ns, err)
		}
	}

	return targets, targetPatterns, nil
}

// Returns an annotation as "namespace/name" format
func resolveAnnotation(object *metav1.ObjectMeta, annotation string) (string, bool) {
	if val, ok := object.Annotations[annotation]; !ok {
		return "", false
	} else if strings.ContainsAny(val, "/") {
		return val, true
	} else {
		return fmt.Sprintf("%s/%s", object.Namespace, val), true
	}
}

// Returns true if the annotation from the object references the other object
func annotationRefersTo(object *metav1.ObjectMeta, annotation string, reference *metav1.ObjectMeta) bool {
	if val, ok := object.Annotations[annotation]; !ok {
		return false
	} else if v := strings.SplitN(val, "/", 2); len(v) == 2 {
		return v[0] == reference.Namespace && v[1] == reference.Name
	} else {
		return object.Namespace == reference.Namespace && val == reference.Name
	}
}
