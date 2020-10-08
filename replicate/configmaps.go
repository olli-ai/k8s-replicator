package replicate

import (
	"log"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

var replicatorActions *configMapActions = &configMapActions{}

// NewConfigMapReplicator creates a new config map replicator
func NewConfigMapReplicator(client kubernetes.Interface, resyncPeriod time.Duration, allowAll bool) Replicator {
	repl := objectReplicator{
		replicatorProps: replicatorProps{
			Name:            "config map",
			allowAll:        allowAll,
			client:          client,

			targetsFrom:     make(map[string][]string),
			targetsTo:       make(map[string][]string),

			watchedTargets:  make(map[string][]string),
			watchedPatterns: make(map[string][]targetPattern),
		},
		replicatorActions: replicatorActions,
	}
	// init the namespace informer
	namespaceStore, namespaceController := cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(lo metav1.ListOptions) (runtime.Object, error) {
				list, err := client.CoreV1().Namespaces().List(lo)
				if err != nil {
					return list, err
				}
				// populate the store already, to avoid believing some items are deleted
				copy := make([]interface{}, len(list.Items))
				for index := range list.Items {
					copy[index] = &list.Items[index]
				}
				repl.namespaceStore.Replace(copy, "init")
				return list, err
			},
			WatchFunc: func(lo metav1.ListOptions) (watch.Interface, error) {
				return client.CoreV1().Namespaces().Watch(lo)
			},
		},
		&v1.Namespace{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    repl.NamespaceAdded,
			UpdateFunc: func(old interface{}, new interface{}) {},
			DeleteFunc: func(obj interface{}) {},
		},
	)

	repl.namespaceStore = namespaceStore
	repl.namespaceController = namespaceController
	// init the object informer
	objectStore, objectController := cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(lo metav1.ListOptions) (runtime.Object, error) {
				list, err := client.CoreV1().ConfigMaps("").List(lo)
				if err != nil {
					return list, err
				}
				// populate the store already, to avoid believing some items are deleted
				copy := make([]interface{}, len(list.Items))
				for index := range list.Items {
					copy[index] = &list.Items[index]
				}
				repl.objectStore.Replace(copy, "init")
				return list, err
			},
			WatchFunc: func(lo metav1.ListOptions) (watch.Interface, error) {
				return client.CoreV1().ConfigMaps("").Watch(lo)
			},
		},
		&v1.ConfigMap{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    repl.ObjectAdded,
			UpdateFunc: func(old interface{}, new interface{}) { repl.ObjectAdded(new) },
			DeleteFunc: repl.ObjectDeleted,
		},
	)

	repl.objectStore = objectStore
	repl.objectController = objectController

	return &repl
}

type configMapActions struct {}

func (*configMapActions) getMeta(object interface{}) *metav1.ObjectMeta {
	return &object.(*v1.ConfigMap).ObjectMeta
}

func (*configMapActions) update(r *replicatorProps, object interface{}, sourceObject interface{}, annotations map[string]string) error {
	sourceConfigMap := sourceObject.(*v1.ConfigMap)
	// copy the configMap
	configMap := object.(*v1.ConfigMap).DeepCopy()
	// set the annotations
	configMap.Annotations = annotations
	// copy the data
	if sourceConfigMap.Data != nil {
		configMap.Data = make(map[string]string)
		for key, value := range sourceConfigMap.Data {
			configMap.Data[key] = value
		}
	} else {
		configMap.Data = nil
	}
	// copy the binary data
	if sourceConfigMap.BinaryData != nil {
		configMap.BinaryData = make(map[string][]byte)
		for key, value := range sourceConfigMap.BinaryData {
			newValue := make([]byte, len(value))
			copy(newValue, value)
			configMap.BinaryData[key] = newValue
		}
	} else {
		configMap.BinaryData = nil
	}

	log.Printf("updating config map %s/%s", configMap.Namespace, configMap.Name)
	// update the configMap
	s, err := r.client.CoreV1().ConfigMaps(configMap.Namespace).Update(configMap)
	if err != nil {
		log.Printf("error while updating config map %s/%s: %s", configMap.Namespace, configMap.Name, err)
		return err
	}
	// update the object store in advance, to avoid being disturbed later
	r.objectStore.Update(s)
	return nil
}

func (*configMapActions) clear(r *replicatorProps, object interface{}, annotations map[string]string) error {
	// copy the configMap
	configMap := object.(*v1.ConfigMap).DeepCopy()
	// set the annotations
	configMap.Annotations = annotations
	// clear the data
	configMap.Data = nil
	// clear the binary data
	configMap.BinaryData = nil

	log.Printf("clearing config map %s/%s", configMap.Namespace, configMap.Name)
	// update the configMap
	s, err := r.client.CoreV1().ConfigMaps(configMap.Namespace).Update(configMap)
	if err != nil {
		log.Printf("error while clearing config map %s/%s", configMap.Namespace, configMap.Name)
		return err
	}
	// update the object store in advance, to avoid being disturbed later
	r.objectStore.Update(s)
	return nil
}

func (*configMapActions) install(r *replicatorProps, meta *metav1.ObjectMeta, sourceObject interface{}, dataObject interface{}) error {
	sourceConfigMap := sourceObject.(*v1.ConfigMap)
	// create a new configMap
	configMap := v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       sourceConfigMap.Kind,
			APIVersion: sourceConfigMap.APIVersion,
		},
		ObjectMeta: *meta,
	}
	// if there is data
	if dataObject != nil {
		dataConfigMap := dataObject.(*v1.ConfigMap)
		// copy the data
		if dataConfigMap.Data != nil {
			configMap.Data = make(map[string]string)
			for key, value := range dataConfigMap.Data {
				configMap.Data[key] = value
			}
		}
		// copy the binary data
		if dataConfigMap.BinaryData != nil {
			configMap.BinaryData = make(map[string][]byte)
			for key, value := range dataConfigMap.BinaryData {
				newValue := make([]byte, len(value))
				copy(newValue, value)
				configMap.BinaryData[key] = newValue
			}
		}
	}

	log.Printf("installing config map %s/%s", configMap.Namespace, configMap.Name)

	var s *v1.ConfigMap
	var err error
	if configMap.ResourceVersion == "" {
		// create the configMap
		s, err = r.client.CoreV1().ConfigMaps(configMap.Namespace).Create(&configMap)
	} else {
		// update the configMap
		s, err = r.client.CoreV1().ConfigMaps(configMap.Namespace).Update(&configMap)
	}

	if err != nil {
		log.Printf("error while installing config map %s/%s: %s", configMap.Namespace, configMap.Name, err)
		return err
	}
	// update the object store in advance, to avoid being disturbed later
	r.objectStore.Update(s)
	return nil
}

func (*configMapActions) delete(r *replicatorProps, object interface{}) error {
	configMap := object.(*v1.ConfigMap)
	log.Printf("deleting config map %s/%s", configMap.Namespace, configMap.Name)
	// prepare the delete options
	options := metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{
			ResourceVersion: &configMap.ResourceVersion,
		},
	}
	// delete the configMap
	err := r.client.CoreV1().ConfigMaps(configMap.Namespace).Delete(configMap.Name, &options)
	if err != nil {
		log.Printf("error while deleting config map %s/%s: %s", configMap.Namespace, configMap.Name, err)
		return err
	}
	// update the object store in advance, to avoid being disturbed later
	r.objectStore.Delete(configMap)
	return nil
}
