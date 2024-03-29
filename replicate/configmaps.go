package replicate

import (
	"log"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

var _configMapActions *configMapActions = &configMapActions{}

// NewConfigMapReplicator creates a new config map replicator
func NewConfigMapReplicator(client kubernetes.Interface, options ReplicatorOptions, resyncPeriod time.Duration) Replicator {
	repl := ObjectReplicator{
		ReplicatorProps:   NewReplicatorProps(client, "configMap", options),
		ReplicatorActions: _configMapActions,
	}
	configmaps := client.CoreV1().ConfigMaps("")
	listWatch := cache.ListWatch{
		ListFunc: func(lo metav1.ListOptions) (runtime.Object, error) {
			return configmaps.List(lo)
		},
		WatchFunc: configmaps.Watch,
	}
	repl.InitStores(&listWatch, &v1.ConfigMap{}, resyncPeriod)
	return &repl
}

type configMapActions struct {}

func (*configMapActions) GetMeta(object interface{}) *metav1.ObjectMeta {
	return &object.(*v1.ConfigMap).ObjectMeta
}

func copyConfigMapData(configMap *v1.ConfigMap, sourceObject interface{}) {
	if sourceObject != nil {
		sourceConfigMap := sourceObject.(*v1.ConfigMap)
		// copy the data
		if sourceConfigMap.Data != nil {
			configMap.Data = make(map[string]string, len(sourceConfigMap.Data))
			for key, value := range sourceConfigMap.Data {
				configMap.Data[key] = value
			}
		} else {
			configMap.Data = nil
		}
		// copy the binary data
		if sourceConfigMap.BinaryData != nil {
			configMap.BinaryData = make(map[string][]byte, len(sourceConfigMap.BinaryData))
			for key, value := range sourceConfigMap.BinaryData {
				newValue := make([]byte, len(value))
				copy(newValue, value)
				configMap.BinaryData[key] = newValue
			}
		} else {
			configMap.BinaryData = nil
		}
	}
}

func (*configMapActions) Update(client kubernetes.Interface, object interface{}, sourceObject interface{}, annotations map[string]string) (interface{}, error) {
	// copy the configMap
	configMap := object.(*v1.ConfigMap).DeepCopy()
	// set the annotations
	configMap.Annotations = annotations
	// copy the data
	copyConfigMapData(configMap, sourceObject)

	log.Printf("updating configMap %s/%s", configMap.Namespace, configMap.Name)
	// update the configMap
	update, err := client.CoreV1().ConfigMaps(configMap.Namespace).Update(configMap)
	if err != nil {
		log.Printf("error while updating configMap %s/%s: %s", configMap.Namespace, configMap.Name, err)
	}
	return update, err
}

func (*configMapActions) Clear(client kubernetes.Interface, object interface{}, annotations map[string]string) (interface{}, error) {
	// copy the configMap
	configMap := object.(*v1.ConfigMap).DeepCopy()
	// set the annotations
	configMap.Annotations = annotations
	// clear the data
	configMap.Data = nil
	// clear the binary data
	configMap.BinaryData = nil

	log.Printf("clearing configMap %s/%s", configMap.Namespace, configMap.Name)
	// update the configMap
	update, err := client.CoreV1().ConfigMaps(configMap.Namespace).Update(configMap)
	if err != nil {
		log.Printf("error while clearing configMap %s/%s", configMap.Namespace, configMap.Name)
	}
	return update, err
}

func (*configMapActions) Install(client kubernetes.Interface, meta *metav1.ObjectMeta, sourceObject interface{}, dataObject interface{}) (interface{}, error) {
	// sourceConfigMap := sourceObject.(*v1.ConfigMap)
	// create a new configMap
	configMap := v1.ConfigMap{
		ObjectMeta: *meta,
	}
	// copy the data
	copyConfigMapData(&configMap, dataObject)

	log.Printf("installing configMap %s/%s", configMap.Namespace, configMap.Name)

	var update *v1.ConfigMap
	var err error
	if configMap.ResourceVersion == "" {
		// create the configMap
		update, err = client.CoreV1().ConfigMaps(configMap.Namespace).Create(&configMap)
	} else {
		// update the configMap
		update, err = client.CoreV1().ConfigMaps(configMap.Namespace).Update(&configMap)
	}

	if err != nil {
		log.Printf("error while installing configMap %s/%s: %s", configMap.Namespace, configMap.Name, err)
	}
	return update, err
}

func (*configMapActions) Delete(client kubernetes.Interface, object interface{}) error {
	configMap := object.(*v1.ConfigMap)
	log.Printf("deleting configMap %s/%s", configMap.Namespace, configMap.Name)
	// prepare the delete options
	options := metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{
			ResourceVersion: &configMap.ResourceVersion,
		},
	}
	// delete the configMap
	err := client.CoreV1().ConfigMaps(configMap.Namespace).Delete(configMap.Name, &options)
	if err != nil {
		log.Printf("error while deleting configMap %s/%s: %s", configMap.Namespace, configMap.Name, err)
	}
	return err
}
