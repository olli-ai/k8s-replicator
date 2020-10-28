package replicate

import (
	"fmt"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

func init() {
	PrefixAnnotations("test-prefix")
}

type M = map[string]string
type MB = map[string][]byte

type actionsWatcher struct{
	Actions []testing.Action
}

type CreateAction = testing.CreateAction
type UpdateAction = testing.UpdateAction
type DeleteAction = testing.DeleteAction

func (w *actionsWatcher) react(action testing.Action) (bool, runtime.Object, error) {
	w.Actions = append(w.Actions, action)
	return false, nil, nil
}

func createReplicator(actions ReplicatorActions, namespaces ...string) (*ReplicatorProps, *actionsWatcher) {
	objects := []runtime.Object{}
	for _, ns := range namespaces {
		objects = append(objects, &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
			},
		})
	}
	client := fake.NewSimpleClientset(objects...)
	watcher := &actionsWatcher{}
	client.PrependReactor("*", "*", watcher.react)
	store := cache.NewStore(func(object interface{}) (string, error) {
		meta := actions.GetMeta(object)
		return fmt.Sprintf("%s/%s", meta.Namespace, meta.Name), nil
	})
	return &ReplicatorProps{
		client: client,
		objectStore: store,
	}, watcher
}
