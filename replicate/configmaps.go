package replicate

import (
	"log"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

var _configMapActions *configMapActions = &configMapActions{}

// NewConfigMapReplicator creates a new config map replicator
func NewConfigMapReplicator(client kubernetes.Interface, options ReplicatorOptions, resyncPeriod time.Duration) Replicator {
	repl := ObjectReplicator{
		ReplicatorProps:   NewReplicatorProps(client, "configMap", options),
		ReplicatorActions: _configMapActions,
	}
	listFunc := func(lo metav1.ListOptions) (runtime.Object, []interface{}, error) {
		list, err := client.CoreV1().ConfigMaps("").List(lo)
		if err != nil {
			return list, nil, err
		}
		copy := make([]interface{}, len(list.Items))
		for index := range list.Items {
			copy[index] = &list.Items[index]
		}
		return list, copy, err
	}
	watchFunc := client.CoreV1().ConfigMaps("").Watch
	repl.InitStores(listFunc, watchFunc, &v1.ConfigMap{}, resyncPeriod)
	return &repl
}

type configMapActions struct {}

func (*configMapActions) GetMeta(object interface{}) *metav1.ObjectMeta {
	return &object.(*v1.ConfigMap).ObjectMeta
}

func (*configMapActions) Update(client kubernetes.Interface, object interface{}, sourceObject interface{}, annotations map[string]string) (interface{}, error) {
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
	update, err := client.CoreV1().ConfigMaps(configMap.Namespace).Update(configMap)
	if err != nil {
		log.Printf("error while updating config map %s/%s: %s", configMap.Namespace, configMap.Name, err)
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

	log.Printf("clearing config map %s/%s", configMap.Namespace, configMap.Name)
	// update the configMap
	update, err := client.CoreV1().ConfigMaps(configMap.Namespace).Update(configMap)
	if err != nil {
		log.Printf("error while clearing config map %s/%s", configMap.Namespace, configMap.Name)
	}
	return update, err
}

func (*configMapActions) Install(client kubernetes.Interface, meta *metav1.ObjectMeta, sourceObject interface{}, dataObject interface{}) (interface{}, error) {
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
		log.Printf("error while installing config map %s/%s: %s", configMap.Namespace, configMap.Name, err)
	}
	return update, err
}

func (*configMapActions) Delete(client kubernetes.Interface, object interface{}) error {
	configMap := object.(*v1.ConfigMap)
	log.Printf("deleting config map %s/%s", configMap.Namespace, configMap.Name)
	// prepare the delete options
	options := metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{
			ResourceVersion: &configMap.ResourceVersion,
		},
	}
	// delete the configMap
	err := client.CoreV1().ConfigMaps(configMap.Namespace).Delete(configMap.Name, &options)
	if err != nil {
		log.Printf("error while deleting config map %s/%s: %s", configMap.Namespace, configMap.Name, err)
	}
	return err
}
