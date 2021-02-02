package replicate

import (
	"crypto/rand"
	"log"
	"math/big"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

var _secretActions *secretActions = &secretActions{}

// NewSecretReplicator creates a new secret replicator
func NewSecretReplicator(client kubernetes.Interface, options ReplicatorOptions, resyncPeriod time.Duration) Replicator {
	repl := ObjectReplicator{
		ReplicatorProps:   NewReplicatorProps(client, "secret", options),
		ReplicatorActions: _secretActions,
	}
	secrets := client.CoreV1().Secrets("")
	listWatch := cache.ListWatch{
		ListFunc: func(lo metav1.ListOptions) (runtime.Object, error) {
			return secrets.List(lo)
		},
		WatchFunc: secrets.Watch,
	}
	repl.InitStores(&listWatch, &v1.Secret{}, resyncPeriod)
	return &repl
}

type secretActions struct {}

func (*secretActions) GetMeta(object interface{}) *metav1.ObjectMeta {
	return &object.(*v1.Secret).ObjectMeta
}

const passwordChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
const passwordLength = 128

type emptySecretFuncsType = map[v1.SecretType] func() (map[string]string, error)
// functions for generating empty data for specific secret types
var emptySecretFuncs emptySecretFuncsType = emptySecretFuncsType{
	v1.SecretTypeDockercfg: func() (map[string]string, error) {
		return map[string]string{
			// This field is checked to be JSON
			v1.DockerConfigKey: "{}",
		}, nil
	},
	v1.SecretTypeDockerConfigJson: func() (map[string]string, error) {
		return map[string]string{
			// This field is checked to be JSON
			v1.DockerConfigJsonKey: "{}",
		}, nil
	},
	v1.SecretTypeBasicAuth: func() (map[string]string, error) {
		// generate a long random password instead of an empty one for safety
		max := big.NewInt(int64(len(passwordChars)))
		buf := make([]byte, passwordLength)
		for i := 0; i < passwordLength; i++ {
			char, err := rand.Int(rand.Reader, max)
			if err != nil {
				return nil, err
			}
			buf[i] = passwordChars[char.Int64()]
		}
		return map[string]string{
			v1.BasicAuthUsernameKey: "",
			// could use an empty password, safer to use a long random one
			v1.BasicAuthPasswordKey: string(buf),
		}, nil
	},
	v1.SecretTypeSSHAuth: func() (map[string]string, error) {
		return map[string]string{
			// this field is checked to be non empty
			v1.SSHAuthPrivateKey: "empty",
		}, nil
	},
	v1.SecretTypeTLS: func() (map[string]string, error) {
		return map[string]string{
			v1.TLSCertKey: "",
			v1.TLSPrivateKeyKey: "",
		}, nil
	},
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
	if emptyFunc, ok := emptySecretFuncs[secret.Type]; ok {
		var err error
		secret.StringData, err = emptyFunc()
		if err != nil {
			return nil, err
		}
	}

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
	} else if emptyFunc, ok := emptySecretFuncs[secret.Type]; ok {
		var err error
		secret.StringData, err = emptyFunc()
		if err != nil {
			return nil, err
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
