package replicate

import (
	"log"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

var _secretActions *secretActions = &secretActions{}

// NewSecretReplicator creates a new secret replicator
func NewSecretReplicator(client kubernetes.Interface, options ReplicatorOptions, resyncPeriod time.Duration) Replicator {
	repl := ObjectReplicator{
		ReplicatorProps:   NewReplicatorProps(client, "secret", options),
		ReplicatorActions: _secretActions,
	}
	listFunc := func(lo metav1.ListOptions) (runtime.Object, []interface{}, error) {
		list, err := client.CoreV1().Secrets("").List(lo)
		if err != nil {
			return list, nil, err
		}
		copy := make([]interface{}, len(list.Items))
		for index := range list.Items {
			copy[index] = &list.Items[index]
		}
		return list, copy, err
	}
	watchFunc := client.CoreV1().Secrets("").Watch
	repl.InitStores(listFunc, watchFunc, &v1.Secret{}, resyncPeriod)
	return &repl
}

type secretActions struct {}

func (*secretActions) GetMeta(object interface{}) *metav1.ObjectMeta {
	return &object.(*v1.Secret).ObjectMeta
}

func (*secretActions) Update(client kubernetes.Interface, object interface{}, sourceObject interface{}, annotations map[string]string) (interface{}, error) {
	sourceSecret := sourceObject.(*v1.Secret)
	// copy the secret
	secret := object.(*v1.Secret).DeepCopy()
	// set the annotations
	secret.Annotations = annotations
	// copy the data
	if sourceSecret.Data != nil {
		secret.Data = make(map[string][]byte)
		for key, value := range sourceSecret.Data {
			newValue := make([]byte, len(value))
			copy(newValue, value)
			secret.Data[key] = newValue
		}
	} else {
		secret.Data = nil
	}

	log.Printf("updating secret %s/%s", secret.Namespace, secret.Name)
	// update the secret
	update, err := client.CoreV1().Secrets(secret.Namespace).Update(secret)
	if err != nil {
		log.Printf("error while updating secret %s/%s: %s", secret.Namespace, secret.Name, err)
	}
	return update, err
}

func (*secretActions) Clear(client kubernetes.Interface, object interface{}, annotations map[string]string) (interface{}, error) {
	// copy the secret
	secret := object.(*v1.Secret).DeepCopy()
	// set the annotations
	secret.Annotations = annotations
	// clear the data
	secret.Data = nil

	log.Printf("clearing secret %s/%s", secret.Namespace, secret.Name)
	// update the secret
	update, err := client.CoreV1().Secrets(secret.Namespace).Update(secret)
	if err != nil {
		log.Printf("error while clearing secret %s/%s", secret.Namespace, secret.Name)
	}
	return update, err
}

func (*secretActions) Install(client kubernetes.Interface, meta *metav1.ObjectMeta, sourceObject interface{}, dataObject interface{}) (interface{}, error) {
	sourceSecret := sourceObject.(*v1.Secret)
	// create a new secret
	secret := v1.Secret{
		Type: sourceSecret.Type,
		TypeMeta: metav1.TypeMeta{
			Kind:       sourceSecret.Kind,
			APIVersion: sourceSecret.APIVersion,
		},
		ObjectMeta: *meta,
	}
	// if there is data
	if dataObject != nil {
		dataSecret := dataObject.(*v1.Secret)
		// copy the data
		if dataSecret.Data != nil {
			secret.Data = make(map[string][]byte)
			for key, value := range dataSecret.Data {
				newValue := make([]byte, len(value))
				copy(newValue, value)
				secret.Data[key] = newValue
			}
		}
	}

	log.Printf("installing secret %s/%s", secret.Namespace, secret.Name)

	var update *v1.Secret
	var err error
	if secret.ResourceVersion == "" {
		// create the secret
		update, err = client.CoreV1().Secrets(secret.Namespace).Create(&secret)
	} else {
		// update the secret
		update, err = client.CoreV1().Secrets(secret.Namespace).Update(&secret)
	}

	if err != nil {
		log.Printf("error while installing secret %s/%s: %s", secret.Namespace, secret.Name, err)
	}
	return update, err
}

func (*secretActions) Delete(client kubernetes.Interface, object interface{}) error {
	secret := object.(*v1.Secret)
	log.Printf("deleting secret %s/%s", secret.Namespace, secret.Name)
	// prepare the delete options
	options := metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{
			ResourceVersion: &secret.ResourceVersion,
		},
	}
	// delete the secret
	err := client.CoreV1().Secrets(secret.Namespace).Delete(secret.Name, &options)
	if err != nil {
		log.Printf("error while deleting secret %s/%s: %s", secret.Namespace, secret.Name, err)
	}
	return err
}
