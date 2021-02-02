package replicate

import (
	"fmt"
	"log"
	"strconv"
	"testing"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testObject struct{
	Type string
	Data string
	Meta metav1.ObjectMeta
}

func (o *testObject) Refresh(actions *testActions) *testObject {
	out := &testObject{
		Type: o.Type,
		Data: o.Data,
		Meta: *o.Meta.DeepCopy(),
	}
	out.Meta.ResourceVersion = strconv.Itoa(actions.Incr)
	actions.Incr ++
	return out
}

type testAction struct{
	Action   string
	Conflict bool
	Object   testObject
}

type testActions struct {
	T       *testing.T
	Store   cache.Store
	Incr    int
	Actions []*testAction
}

func hasConflict(a *testActions, meta *metav1.ObjectMeta) (bool, error) {
	current, ok, err := a.Store.Get(&testObject{
		Meta: *meta,
	})
	if err != nil {
		return false, err
	}
	if !ok {
		return meta.ResourceVersion != "", nil
	}
	return meta.ResourceVersion != current.(*testObject).Meta.ResourceVersion, nil
}

func (*testActions) GetMeta(object interface{}) *metav1.ObjectMeta {
	return &object.(*testObject).Meta
}

func (a *testActions) Update(client kubernetes.Interface, object interface{}, sourceObject interface{}, annotations map[string]string) (interface{}, error) {
	target := object.(*testObject)
	source := sourceObject.(*testObject)
	conflict, err := hasConflict(a, &target.Meta)
	require.NoError(a.T, err)
	action := &testAction{
		Action: "update",
		Conflict: conflict,
		Object: testObject{
			Type: target.Type,
			Data: source.Data,
			Meta: *target.Meta.DeepCopy(),
		},
	}
	m := map[string]string{}
	for k, v := range annotations {
		m[k] = v
	}
	action.Object.Meta.Annotations = m
	a.Actions = append(a.Actions, action)
	if conflict {
		log.Printf("update conflict %s/%s", target.Meta.Namespace, target.Meta.Name)
		return nil, fmt.Errorf("conflict %s/%s", target.Meta.Namespace, target.Meta.Name)
	}
	log.Printf("updating test %s/%s with data \"%s\"", target.Meta.Namespace, target.Meta.Name, source.Data)
	return action.Object.Refresh(a), nil
}

func (a *testActions) Clear(client kubernetes.Interface, object interface{}, annotations map[string]string) (interface{}, error) {
	target := object.(*testObject)
	conflict, err := hasConflict(a, &target.Meta)
	require.NoError(a.T, err)
	action := &testAction{
		Action: "clear",
		Conflict: conflict,
		Object: testObject{
			Type: target.Type,
			Data: "",
			Meta: *target.Meta.DeepCopy(),
		},
	}
	m := map[string]string{}
	for k, v := range annotations {
		m[k] = v
	}
	action.Object.Meta.Annotations = m
	a.Actions = append(a.Actions, action)
	if conflict {
		log.Printf("clear conflict %s/%s", target.Meta.Namespace, target.Meta.Name)
		return nil, fmt.Errorf("conflict %s/%s", target.Meta.Namespace, target.Meta.Name)
	}
	log.Printf("clearing test %s/%s", target.Meta.Namespace, target.Meta.Name)
	return action.Object.Refresh(a), nil
}

func (a *testActions) Install(client kubernetes.Interface, meta *metav1.ObjectMeta, sourceObject interface{}, dataObject interface{}) (interface{}, error) {
	source := sourceObject.(*testObject)
	data := ""
	if dataObject != nil {
		data = dataObject.(*testObject).Data
	}
	conflict, err := hasConflict(a, meta)
	require.NoError(a.T, err)
	action := &testAction{
		Action: "install",
		Conflict: conflict,
		Object: testObject{
			Type: source.Type,
			Data: data,
			Meta: *meta.DeepCopy(),
		},
	}
	a.Actions = append(a.Actions, action)
	if conflict {
		log.Printf("install conflict %s/%s", meta.Namespace, meta.Name)
		return nil, fmt.Errorf("conflict %s/%s", meta.Namespace, meta.Name)
	}
	log.Printf("installing test %s/%s with type \"%s\" and data \"%s\"", meta.Namespace, meta.Name, source.Type, data)
	return action.Object.Refresh(a), nil
}

func (a *testActions) Delete(client kubernetes.Interface, object interface{}) error {
	target := object.(*testObject)
	conflict, err := hasConflict(a, &target.Meta)
	require.NoError(a.T, err)
	action := &testAction{
		Action: "delete",
		Conflict: conflict,
		Object: testObject{
			Type: "",
			Data: "",
			Meta: *target.Meta.DeepCopy(),
		},
	}
	a.Actions = append(a.Actions, action)
	if conflict {
		log.Printf("delete conflict %s/%s", target.Meta.Namespace, target.Meta.Name)
		return fmt.Errorf("conflict %s/%s", target.Meta.Namespace, target.Meta.Name)
	}
	log.Printf("deleting test %s/%s", target.Meta.Namespace, target.Meta.Name)
	require.NoError(a.T, a.Store.Delete(&action.Object))
	return nil
}

func testKey(object interface{}) (string, error) {
	meta := object.(*testObject).Meta
	return fmt.Sprintf("%s/%s", meta.Namespace, meta.Name), nil
}

func namespaceKey(object interface{}) (string, error) {
	return object.(*v1.Namespace).Name, nil
}

func createTestReplicator(t *testing.T, options ReplicatorOptions, namespaces ...string) *ObjectReplicator {
	objectStore := cache.NewStore(testKey)
	namespaceStore := cache.NewStore(namespaceKey)
	replicator := &ObjectReplicator{
		ReplicatorProps: NewReplicatorProps(nil, "test", options),
		ReplicatorActions: &testActions{
			T:     t,
			Store: objectStore,
		},
	}
	replicator.objectStore = objectStore
	replicator.namespaceStore = namespaceStore
	if len(namespaces) > 0 {
		objects := []interface{}{}
		for _, ns := range namespaces {
			objects = append(objects, &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ns,
				},
			})
		}
		require.NoError(t, replicator.namespaceStore.Replace(objects, ""))
	}
	return replicator
}

func addNamespace(r *ObjectReplicator, namespace string) *v1.Namespace {
	object := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	require.NoError(r.ReplicatorActions.(*testActions).T, r.namespaceStore.Update(object))
	return object
}

func deleteNamespace(r *ObjectReplicator, namespace string) *v1.Namespace {
	object := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	require.NoError(r.ReplicatorActions.(*testActions).T, r.namespaceStore.Delete(object))
	return object
}

func getObject(r *ObjectReplicator, namespace string, name string) *testObject {
	object, ok, err := r.objectStore.GetByKey(fmt.Sprintf("%s/%s", namespace, name))
	require.NoError(r.ReplicatorActions.(*testActions).T, err)
	if !ok {
		return nil
	}
	return object.(*testObject)
}

func updateObject(r *ObjectReplicator, namespace string, name string, annotations map[string]string) *testObject {
	if annotations == nil {
		annotations = getObject(r, namespace, name).Meta.Annotations
	}
	copy := map[string]string{}
	for k, v := range annotations {
		copy[k] = v
	}
	actions := r.ReplicatorActions.(*testActions)
	object := &testObject{
		Type: strconv.Itoa(actions.Incr),
		Data: strconv.Itoa(actions.Incr),
		Meta: metav1.ObjectMeta{
			Namespace:       namespace,
			Name:            name,
			Annotations:     copy,
			ResourceVersion: strconv.Itoa(actions.Incr),
		},
	}
	actions.Incr ++
	require.NoError(r.ReplicatorActions.(*testActions).T, r.objectStore.Update(object))
	return object
}

func deleteObject(r *ObjectReplicator, namespace string, name string) *testObject {
	object := getObject(r, namespace, name)
	require.NoError(r.ReplicatorActions.(*testActions).T, r.objectStore.Delete(object))
	return object
}

func assertAction(t *testing.T, r *ObjectReplicator, index int, action *testAction) {
	actions := r.ReplicatorActions.(*testActions).Actions
	prefix := fmt.Sprintf("[%d].", index)
	if assert.Truef(t, len(actions) > index, "len(actions) > %d", index) &&
			assert.Equal(t, action.Action, actions[index].Action, prefix + "Action") &&
			assert.Equal(t, action.Object.Meta.Namespace, actions[index].Object.Meta.Namespace, prefix + "Object.Meta.Namespace") &&
			assert.Equal(t, action.Object.Meta.Name, actions[index].Object.Meta.Name, prefix + "Object.Meta.Name") {
		assert.Equal(t, action.Conflict, actions[index].Conflict, prefix + "Conflict")
		assert.Equal(t, action.Object.Type, actions[index].Object.Type, prefix + "Object.Type")
		assert.Equal(t, action.Object.Data, actions[index].Object.Data, prefix + "Object.Data")
		assert.Equal(t, action.Object.Meta.ResourceVersion, actions[index].Object.Meta.ResourceVersion, prefix + "Object.Meta.ResourceVersion")
		testAnnotations := actions[index].Object.Meta.Annotations
		for k, v := range action.Object.Meta.Annotations {
			assert.Equalf(t, v, testAnnotations[k], "[%v]Object.Meta.Annotations[\"%v\"]", index, k)
		}
		testLabels := actions[index].Object.Meta.Labels
		for k, v := range action.Object.Meta.Labels {
			assert.Equalf(t, v, testLabels[k], "[%v]Object.Meta.Labels[\"%v\"]", index, k)
		}
	}
}

func assertStore(t *testing.T, r *ObjectReplicator, namespace string, name string, resourceVersion string) {
	object, exists, err := r.objectStore.GetByKey(fmt.Sprintf("%s/%s", namespace, name))
	if assert.NoError(t, err) {
		if resourceVersion == "" {
			assert.Falsef(t, exists, "%s/%s should not exists", namespace, name)
		} else if assert.Truef(t, exists, "%s/%s should exists", namespace, name) {
			if test, ok := object.(*testObject); assert.True(t, ok) {
				assert.Equalf(t, resourceVersion, test.Meta.ResourceVersion,
					"%s/%s ResourceVersion", namespace, name)
			}
		}
	}
}

func requireActionsLength(t *testing.T, r *ObjectReplicator, length int) {
	actions := r.ReplicatorActions.(*testActions).Actions
	require.Equal(t, length, len(actions), "len(actions)")
}

func TestReplicateFrom_simple(t *testing.T) {
	r := createTestReplicator(t, ReplicatorOptions{IgnoreUnknown: true})
	source := updateObject(r, "source-ns", "source", M{
		ReplicationAllowedAnnotation: "true",
		annotationsPrefix + "unknown": "...",
	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 0)
	target := updateObject(r, "target-ns", "target", M{
		ReplicateFromAnnotation: "source-ns/source",
		annotationsPrefix + "unknown": "...",
	})

	r.ObjectAdded(target)
	assertAction(t, r, 0, &testAction{
		Action: "update",
		Object: testObject{
			Type: "1",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "1",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "0",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "2")
	requireActionsLength(t, r, 1)

	r.ObjectAdded(getObject(r, "target-ns", "target"))
	requireActionsLength(t, r, 1)
	source = updateObject(r, "source-ns", "source", nil)

	r.ObjectAdded(source)
	assertAction(t, r, 1, &testAction{
		Action: "update",
		Object: testObject{
			Type: "1",
			Data: "3",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "2",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "3",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "4")
	requireActionsLength(t, r, 2)

	source = deleteObject(r, "source-ns", "source")
	r.ObjectDeleted(source)
	assertAction(t, r, 2, &testAction{
		Action: "clear",
		Object: testObject{
			Type: "1",
			Data: "",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "4",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "5")
	requireActionsLength(t, r, 3)

	source = updateObject(r, "source-ns", "source", M{
		ReplicationAllowedAnnotation: "true",
		annotationsPrefix + "unknown": "...",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 3, &testAction{
		Action: "update",
		Object: testObject{
			Type: "1",
			Data: "6",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "5",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "6",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "7")
	requireActionsLength(t, r, 4)

	target = updateObject(r, "target-ns", "target", M{
		ReplicateFromAnnotation: "source-ns/source",
		annotationsPrefix + "unknown": "...",
		ReplicatedFromVersionAnnotation: "6",
	})
	r.ObjectAdded(target)
	requireActionsLength(t, r, 4)

	target = updateObject(r, "target-ns", "target", M{
		ReplicateFromAnnotation: "source-ns/source",
		annotationsPrefix + "unknown": "...",
		ReplicatedFromVersionAnnotation: "...",
	})
	r.ObjectAdded(target)
	assertAction(t, r, 4, &testAction{
		Action: "update",
		Object: testObject{
			Type: "9",
			Data: "6",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "9",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "6",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "10")
	requireActionsLength(t, r, 5)
}

func TestReplicateFrom_allowed(t *testing.T) {
	r := createTestReplicator(t, ReplicatorOptions{})
	source := updateObject(r, "source-ns", "source", M{})
	target := updateObject(r, "target-ns", "target", M{
		ReplicateFromAnnotation: "source-ns/source",
	})
	r.ObjectAdded(target)
	requireActionsLength(t, r, 0)
	source = updateObject(r, "source-ns", "source", M{
		ReplicationAllowedAnnotation: "true",
	})

	r.ObjectAdded(source)
	assertAction(t, r, 0, &testAction{
		Action: "update",
		Object: testObject{
			Type: "1",
			Data: "2",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "1",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "2",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "3")
	requireActionsLength(t, r, 1)

	source = updateObject(r, "source-ns", "source", M{
		ReplicationAllowedAnnotation: "false",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 1, &testAction{
		Action: "clear",
		Object: testObject{
			Type: "1",
			Data: "",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "3",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "5")
	requireActionsLength(t, r, 2)

	source = updateObject(r, "source-ns", "source", M{
		ReplicationAllowedNsAnnotation: "target-ns",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 2, &testAction{
		Action: "update",
		Object: testObject{
			Type: "1",
			Data: "6",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "5",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "6",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "7")
	requireActionsLength(t, r, 3)

	source = updateObject(r, "source-ns", "source", M{
		ReplicationAllowedNsAnnotation: "other-ns",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 3, &testAction{
		Action: "clear",
		Object: testObject{
			Type: "1",
			Data: "",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "7",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "9")
	requireActionsLength(t, r, 4)

	source = deleteObject(r, "source-ns", "source")
	r.ObjectDeleted(source)
	requireActionsLength(t, r, 4)
}

func TestReplicateFrom_onceSsource(t *testing.T) {
	r := createTestReplicator(t, ReplicatorOptions{AllowAll: true})
	target := updateObject(r, "target-ns", "target", M{
		ReplicateFromAnnotation: "source-ns/source",
	})
	r.ObjectAdded(target)
	requireActionsLength(t, r, 0)
	source := updateObject(r, "source-ns", "source", M{})

	r.ObjectAdded(source)
	assertAction(t, r, 0, &testAction{
		Action: "update",
		Object: testObject{
			Type: "0",
			Data: "1",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "0",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "1",
					ReplicateOnceVersionAnnotation: "",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "2")
	requireActionsLength(t, r, 1)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateOnceAnnotation: "true",
	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 1)
	source = updateObject(r, "source-ns", "source", M{
		ReplicateOnceAnnotation: "true",
		ReplicateOnceVersionAnnotation: "new",
	})

	r.ObjectAdded(source)
	assertAction(t, r, 1, &testAction{
		Action: "update",
		Object: testObject{
			Type: "0",
			Data: "4",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "2",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "4",
					ReplicateOnceVersionAnnotation: "new",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "5")
	requireActionsLength(t, r, 2)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateOnceAnnotation: "true",
		ReplicateOnceVersionAnnotation: "new",
	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 2)
	source = updateObject(r, "source-ns", "source", M{
		ReplicateOnceVersionAnnotation: "new",
	})

	r.ObjectAdded(source)
	assertAction(t, r, 2, &testAction{
		Action: "update",
		Object: testObject{
			Type: "0",
			Data: "7",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "5",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "7",
					ReplicateOnceVersionAnnotation: "new",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "8")
	requireActionsLength(t, r, 3)
}

func TestReplicateFrom_onceTtarget(t *testing.T) {
	r := createTestReplicator(t, ReplicatorOptions{AllowAll: true})
	source := updateObject(r, "source-ns", "source", M{})
	r.ObjectAdded(source)
	target := updateObject(r, "target-ns", "target", M{
		ReplicateFromAnnotation: "source-ns/source",
		ReplicateOnceAnnotation: "true",
	})

	r.ObjectAdded(target)
	assertAction(t, r, 0, &testAction{
		Action: "update",
		Object: testObject{
			Type: "1",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "1",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "0",
					ReplicateOnceAnnotation: "true",
					ReplicateOnceVersionAnnotation: "",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "2")
	requireActionsLength(t, r, 1)

	source = updateObject(r, "source-ns", "source", M{})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 1)
	source = updateObject(r, "source-ns", "source", M{
		ReplicateOnceVersionAnnotation: "",
	})

	r.ObjectAdded(source)
	assertAction(t, r, 1, &testAction{
		Action: "update",
		Object: testObject{
			Type: "1",
			Data: "4",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "2",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "4",
					ReplicateOnceAnnotation: "true",
					ReplicateOnceVersionAnnotation: "",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "5")
	requireActionsLength(t, r, 2)
	source = updateObject(r, "source-ns", "source", M{
		ReplicateOnceVersionAnnotation: "other",
	})

	r.ObjectAdded(source)
	assertAction(t, r, 2, &testAction{
		Action: "update",
		Object: testObject{
			Type: "1",
			Data: "6",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "5",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "6",
					ReplicateOnceAnnotation: "true",
					ReplicateOnceVersionAnnotation: "other",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "7")
	requireActionsLength(t, r, 3)
}

func TestReplicateFrom_invalid(t *testing.T) {
	r := createTestReplicator(t, ReplicatorOptions{})
	target := updateObject(r, "target-ns", "target", M{
		ReplicateFromAnnotation: "source-ns/source",
	})
	r.ObjectAdded(target)
	requireActionsLength(t, r, 0)
	source := updateObject(r, "source-ns", "source", M{
		ReplicationAllowedAnnotation: "true",
	})

	r.ObjectAdded(source)
	assertAction(t, r, 0, &testAction{
		Action: "update",
		Object: testObject{
			Type: "0",
			Data: "1",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "0",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "1",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "2")
	requireActionsLength(t, r, 1)

	source = updateObject(r, "source-ns", "source", M{
		ReplicationAllowedAnnotation: "true",
		ReplicateOnceAnnotation: "...",
	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 1)
	source = updateObject(r, "source-ns", "source", M{
		ReplicationAllowedAnnotation: "...",
	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 1)
	source = updateObject(r, "source-ns", "source", M{
		ReplicationAllowedAnnotation: "true",
		annotationsPrefix + "other": "true",

	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 1)
	source = updateObject(r, "source-ns", "source", M{
		ReplicationAllowedAnnotation: "true",
	})

	r.ObjectAdded(source)
	assertAction(t, r, 1, &testAction{
		Action: "update",
		Object: testObject{
			Type: "0",
			Data: "6",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "2",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "6",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "7")
	requireActionsLength(t, r, 2)
}

func TestReplicateTo_name(t *testing.T) {
	r := createTestReplicator(t, ReplicatorOptions{
			IgnoreUnknown: true,
			Labels: M{
				"label1": "value1",
				"label2": "value2",
			},
		}, "my-ns")
	source := updateObject(r, "my-ns", "source", M{
		ReplicateToAnnotation: "target",
		annotationsPrefix + "new": "...",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 0, &testAction{
		Action: "install",
		Object: testObject{
			Type: "0",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "my-ns",
				ResourceVersion: "",
				Labels: M{
					"label1": "value1",
					"label2": "value2",
				},
				Annotations: M{
					ReplicatedFromVersionAnnotation: "0",
					ReplicatedByAnnotation: "my-ns/source",
				},
			},
		},
	})
	assertStore(t, r, "my-ns", "target", "1")
	requireActionsLength(t, r, 1)

	source = updateObject(r, "my-ns", "source", M{
		ReplicateToAnnotation: "target",
		annotationsPrefix + "new": "...",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 1, &testAction{
		Action: "install",
		Object: testObject{
			Type: "2",
			Data: "2",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "my-ns",
				ResourceVersion: "1",
				Labels: M{
					"label1": "value1",
					"label2": "value2",
				},
				Annotations: M{
					ReplicatedFromVersionAnnotation: "2",
					ReplicatedByAnnotation: "my-ns/source",
				},
			},
		},
	})
	assertStore(t, r, "my-ns", "target", "3")
	requireActionsLength(t, r, 2)
	r.ObjectAdded(getObject(r, "my-ns", "target"))
	requireActionsLength(t, r, 2)
	r.ObjectAdded(source)
	requireActionsLength(t, r, 2)

	source = updateObject(r, "my-ns", "source", M{
		ReplicateToAnnotation: "other",
	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 4)
	deleteIndex := 2
	installIndex := 3
	if r.ReplicatorActions.(*testActions).Actions[2].Action == "install" {
		installIndex = 2
		deleteIndex = 3
	}
	assertAction(t, r, deleteIndex, &testAction{
		Action: "delete",
		Object: testObject{
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "my-ns",
				ResourceVersion: "3",
			},
		},
	})
	assertStore(t, r, "my-ns", "target", "")
	assertAction(t, r, installIndex, &testAction{
		Action: "install",
		Object: testObject{
			Type: "4",
			Data: "4",
			Meta: metav1.ObjectMeta{
				Name: "other",
				Namespace: "my-ns",
				ResourceVersion: "",
				Labels: M{
					"label1": "value1",
					"label2": "value2",
				},
				Annotations: M{
					ReplicatedFromVersionAnnotation: "4",
					ReplicatedByAnnotation: "my-ns/source",
				},
			},
		},
	})
	assertStore(t, r, "my-ns", "other", "5")

	other := updateObject(r, "my-ns", "other", M{
		ReplicatedFromVersionAnnotation: "4",
		ReplicatedByAnnotation: "my-ns/source",
		annotationsPrefix + "new": "...",
	})
	r.ObjectAdded(other)
	requireActionsLength(t, r, 4)

	other = updateObject(r, "my-ns", "other", M{
		ReplicatedFromVersionAnnotation: "other",
		ReplicatedByAnnotation: "my-ns/source",
		annotationsPrefix + "new": "...",
	})
	r.ObjectAdded(other)
	assertAction(t, r, 4, &testAction{
		Action: "install",
		Object: testObject{
			Type: "4",
			Data: "4",
			Meta: metav1.ObjectMeta{
				Name: "other",
				Namespace: "my-ns",
				ResourceVersion: "7",
				Labels: M{
					"label1": "value1",
					"label2": "value2",
				},
				Annotations: M{
					ReplicatedFromVersionAnnotation: "4",
					ReplicatedByAnnotation: "my-ns/source",
				},
			},
		},
	})
	assertStore(t, r, "my-ns", "other", "8")
	requireActionsLength(t, r, 5)

	source = deleteObject(r, "my-ns", "source")
	r.ObjectDeleted(source)
	assertAction(t, r, 5, &testAction{
		Action: "delete",
		Object: testObject{
			Meta: metav1.ObjectMeta{
				Name: "other",
				Namespace: "my-ns",
				ResourceVersion: "8",
			},
		},
	})
	assertStore(t, r, "my-ns", "other", "")
	requireActionsLength(t, r, 6)
}

func TestReplicateTo_namespaces(t *testing.T) {
	r := createTestReplicator(t, ReplicatorOptions{}, "target-1", "target-2")
	source := updateObject(r, "source-ns", "my-test", M{
		ReplicateToNsAnnotation: "target-[1-3]",
	})
	r.ObjectAdded(source)

	expected := map[string]*testAction{
		"target-1": {
			Action: "install",
			Object: testObject{
				Type: "0",
				Data: "0",
				Meta: metav1.ObjectMeta{
					Name: "my-test",
					Namespace: "target-1",
					ResourceVersion: "",
					Annotations: M{
						ReplicatedFromVersionAnnotation: "0",
					},
				},
			},
		},
		"target-2": {
			Action: "install",
			Object: testObject{
				Type: "0",
				Data: "0",
				Meta: metav1.ObjectMeta{
					Name: "my-test",
					Namespace: "target-2",
					ResourceVersion: "",
					Annotations: M{
						ReplicatedFromVersionAnnotation: "0",
					},
				},
			},
		},
	}
	actions := r.ReplicatorActions.(*testActions).Actions
	resourcesID := 1
	resources := map[string]string{}
	for i:=0; i<2; i++ {
		if len(actions) <= i {
			continue
		}
		ns := actions[i].Object.Meta.Namespace
		if _, ok := expected[ns]; !ok {
			for n := range expected {
				ns = n
				break
			}
		}
		assertAction(t, r, i, expected[ns])
		delete(expected, ns)
		if actions[i].Action != "delete" {
			resources[ns] = strconv.Itoa(resourcesID)
			assertStore(t, r, ns, "my-test", strconv.Itoa(resourcesID))
			resourcesID ++
		} else {
			assertStore(t, r, ns, "my-test", "")
		}
	}
	requireActionsLength(t, r, 2)

	r.NamespaceAdded(addNamespace(r, "target-3"))
	assertAction(t, r, 2, &testAction{
		Action: "install",
		Object: testObject{
			Type: "0",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "my-test",
				Namespace: "target-3",
				ResourceVersion: "",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "0",
				},
			},
		},
	})
	assertStore(t, r, "target-3", "my-test", "3")
	requireActionsLength(t, r, 3)
	r.NamespaceAdded(addNamespace(r, "target-3"))
	r.NamespaceAdded(addNamespace(r, "target-4"))
	requireActionsLength(t, r, 3)

	source = updateObject(r, "source-ns", "my-test", M{
		ReplicateToNsAnnotation: "target-[2-4]",
	})
	r.ObjectAdded(source)

	expected = map[string]*testAction{
		"target-1": {
			Action: "delete",
			Object: testObject{
				Meta: metav1.ObjectMeta{
					Name: "my-test",
					Namespace: "target-1",
					ResourceVersion: resources["target-1"],
				},
			},
		},
		"target-2": {
			Action: "install",
			Object: testObject{
				Type: "4",
				Data: "4",
				Meta: metav1.ObjectMeta{
					Name: "my-test",
					Namespace: "target-2",
					ResourceVersion: resources["target-2"],
					Annotations: M{
						ReplicatedFromVersionAnnotation: "4",
					},
				},
			},
		},
		"target-3": {
			Action: "install",
			Object: testObject{
				Type: "4",
				Data: "4",
				Meta: metav1.ObjectMeta{
					Name: "my-test",
					Namespace: "target-3",
					ResourceVersion: "3",
					Annotations: M{
						ReplicatedFromVersionAnnotation: "4",
					},
				},
			},
		},
		"target-4": {
			Action: "install",
			Object: testObject{
				Type: "4",
				Data: "4",
				Meta: metav1.ObjectMeta{
					Name: "my-test",
					Namespace: "target-4",
					ResourceVersion: "",
					Annotations: M{
						ReplicatedFromVersionAnnotation: "4",
					},
				},
			},
		},
	}
	actions = r.ReplicatorActions.(*testActions).Actions
	resourcesID = 5
	resources = map[string]string{}
	for i:=3; i<3+4; i++ {
		if len(actions) <= i {
			continue
		}
		ns := actions[i].Object.Meta.Namespace
		if _, ok := expected[ns]; !ok {
			for n := range expected {
				ns = n
				break
			}
		}
		assertAction(t, r, i, expected[ns])
		delete(expected, ns)
		if actions[i].Action != "delete" {
			resources[ns] = strconv.Itoa(resourcesID)
			assertStore(t, r, ns, "my-test", strconv.Itoa(resourcesID))
			resourcesID++
		} else {
			assertStore(t, r, ns, "my-test", "")
		}
	}
	requireActionsLength(t, r, 7)

	source = updateObject(r, "source-ns", "my-test", M{
		ReplicateToNsAnnotation: "target-2,target-4",
	})
	r.ObjectAdded(source)

	expected = map[string]*testAction{
		"target-2": {
			Action: "install",
			Object: testObject{
				Type: "8",
				Data: "8",
				Meta: metav1.ObjectMeta{
					Name: "my-test",
					Namespace: "target-2",
					ResourceVersion: resources["target-2"],
					Annotations: M{
						ReplicatedFromVersionAnnotation: "8",
					},
				},
			},
		},
		"target-3": {
			Action: "delete",
			Object: testObject{
				Meta: metav1.ObjectMeta{
					Name: "my-test",
					Namespace: "target-3",
					ResourceVersion: resources["target-3"],
				},
			},
		},
		"target-4": {
			Action: "install",
			Object: testObject{
				Type: "8",
				Data: "8",
				Meta: metav1.ObjectMeta{
					Name: "my-test",
					Namespace: "target-4",
					ResourceVersion: resources["target-4"],
					Annotations: M{
						ReplicatedFromVersionAnnotation: "8",
					},
				},
			},
		},
	}
	actions = r.ReplicatorActions.(*testActions).Actions
	resourcesID = 9
	resources = map[string]string{}
	for i:=7; i<7+3; i++ {
		if len(actions) <= i {
			continue
		}
		ns := actions[i].Object.Meta.Namespace
		if _, ok := expected[ns]; !ok {
			for n := range expected {
				ns = n
				break
			}
		}
		assertAction(t, r, i, expected[ns])
		delete(expected, ns)
		if actions[i].Action != "delete" {
			resources[ns] = strconv.Itoa(resourcesID)
			assertStore(t, r, ns, "my-test", strconv.Itoa(resourcesID))
			resourcesID++
		} else {
			assertStore(t, r, ns, "my-test", "")
		}
	}
	requireActionsLength(t, r, 10)

	source = deleteObject(r, "source-ns", "my-test")
	r.ObjectDeleted(source)

	expected = map[string]*testAction{
		"target-2": {
			Action: "delete",
			Object: testObject{
				Meta: metav1.ObjectMeta{
					Name: "my-test",
					Namespace: "target-2",
					ResourceVersion: resources["target-2"],
				},
			},
		},
		"target-4": {
			Action: "delete",
			Object: testObject{
				Meta: metav1.ObjectMeta{
					Name: "my-test",
					Namespace: "target-4",
					ResourceVersion: resources["target-4"],
				},
			},
		},
	}
	actions = r.ReplicatorActions.(*testActions).Actions
	for i:=10; i<10+2; i++ {
		if len(actions) <= i {
			continue
		}
		ns := actions[i].Object.Meta.Namespace
		if _, ok := expected[ns]; !ok {
			for n := range expected {
				ns = n
				break
			}
		}
		assertAction(t, r, i, expected[ns])
		assertStore(t, r, ns, "my-test", "")
		delete(expected, ns)
	}
	requireActionsLength(t, r, 12)
	r.ObjectDeleted(source)
	requireActionsLength(t, r, 12)
}

func TestReplicateTo_once(t *testing.T) {
	r := createTestReplicator(t, ReplicatorOptions{}, "target-1")
	source := updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-[0-9]+/target",
		ReplicateOnceAnnotation: "true",
	})
	r.ObjectAdded(source)

	assertAction(t, r, 0, &testAction{
		Action: "install",
		Object: testObject{
			Type: "0",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-1",
				ResourceVersion: "",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "0",
				},
			},
		},
	})
	assertStore(t, r, "target-1", "target", "1")
	requireActionsLength(t, r, 1)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-[0-9]+/target",
		ReplicateOnceAnnotation: "true",
	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 1)
	r.ObjectAdded(source)
	requireActionsLength(t, r, 1)

	r.NamespaceAdded(addNamespace(r, "target-2"))

	assertAction(t, r, 1, &testAction{
		Action: "install",
		Object: testObject{
			Type: "2",
			Data: "2",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-2",
				ResourceVersion: "",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "2",
				},
			},
		},
	})
	assertStore(t, r, "target-2", "target", "3")
	requireActionsLength(t, r, 2)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-[0-9]+/target",
		ReplicateOnceAnnotation: "true",
		ReplicateOnceVersionAnnotation: "new",
	})
	r.ObjectAdded(source)

	requireActionsLength(t, r, 4)
	index1 := 2
	index2 := 3
	if r.ReplicatorActions.(*testActions).Actions[2].Object.Meta.Namespace == "target-2" {
		index2 = 2
		index1 = 3
	}
	assertAction(t, r, index1, &testAction{
		Action: "install",
		Object: testObject{
			Type: "4",
			Data: "4",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-1",
				ResourceVersion: "1",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "4",
					ReplicateOnceVersionAnnotation: "new",
				},
			},
		},
	})
	assertStore(t, r, "target-1", "target", strconv.Itoa(index1 + 3))
	assertAction(t, r, index2, &testAction{
		Action: "install",
		Object: testObject{
			Type: "4",
			Data: "4",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-2",
				ResourceVersion: "3",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "4",
					ReplicateOnceVersionAnnotation: "new",
				},
			},
		},
	})
	assertStore(t, r, "target-2", "target", strconv.Itoa(index2 + 3))

	source = deleteObject(r, "source-ns", "source")
	r.ObjectDeleted(source)

	requireActionsLength(t, r, 6)
	index3 := 4
	index4 := 5
	if r.ReplicatorActions.(*testActions).Actions[4].Object.Meta.Namespace == "target-2" {
		index4 = 4
		index3 = 5
	}
	assertAction(t, r, index3, &testAction{
		Action: "delete",
		Object: testObject{
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-1",
				ResourceVersion: strconv.Itoa(3+index1),
			},
		},
	})
	assertStore(t, r, "target-1", "target", "")
	assertAction(t, r, index4, &testAction{
		Action: "delete",
		Object: testObject{
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-2",
				ResourceVersion: strconv.Itoa(3+index2),
			},
		},
	})
	assertStore(t, r, "target-2", "target", "")
	r.ObjectDeleted(source)
	requireActionsLength(t, r, 6)
}

func TestReplicateTo_annotations(t *testing.T) {
	r := createTestReplicator(t, ReplicatorOptions{}, "target-ns")

	source := updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-ns/target",
		ReplicationAllowedAnnotation: "true",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 0, &testAction{
		Action: "install",
		Object: testObject{
			Type: "0",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "0",
					ReplicationAllowedAnnotation: "true",
					ReplicationAllowedNsAnnotation: "",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "1")
	requireActionsLength(t, r, 1)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-ns/target",
		ReplicationAllowedNsAnnotation: ".*",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 1, &testAction{
		Action: "install",
		Object: testObject{
			Type: "2",
			Data: "2",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "1",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "2",
					ReplicationAllowedAnnotation: "",
					ReplicationAllowedNsAnnotation: ".*",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "3")
	requireActionsLength(t, r, 2)
	r.ObjectAdded(source)
	requireActionsLength(t, r, 2)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-ns/target",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 2, &testAction{
		Action: "install",
		Object: testObject{
			Type: "4",
			Data: "4",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "3",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "4",
					ReplicationAllowedAnnotation: "",
					ReplicationAllowedNsAnnotation: "",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "5")
	requireActionsLength(t, r, 3)
}

func TestReplicateTo_exsists(t *testing.T) {
	r := createTestReplicator(t, ReplicatorOptions{}, "target-1", "target-2")
	source := updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-[0-9]+/target",
	})
	target1 := updateObject(r, "target-1", "target", M{})

	r.ObjectAdded(source)
	r.ObjectAdded(source)
	assertAction(t, r, 0, &testAction{
		Action: "install",
		Object: testObject{
			Type: "0",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-2",
				ResourceVersion: "",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "0",
				},
			},
		},
	})
	assertStore(t, r, "target-2", "target", "2")
	requireActionsLength(t, r, 1)

	target1 = deleteObject(r, "target-1", "target")
	r.ObjectDeleted(target1)
	assertAction(t, r, 1, &testAction{
		Action: "install",
		Object: testObject{
			Type: "0",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-1",
				ResourceVersion: "",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "0",
				},
			},
		},
	})
	assertStore(t, r, "target-1", "target", "3")
	requireActionsLength(t, r, 2)

	target2 := deleteObject(r, "target-2", "target")
	r.ObjectDeleted(target2)
	assertAction(t, r, 2, &testAction{
		Action: "install",
		Object: testObject{
			Type: "0",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-2",
				ResourceVersion: "",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "0",
				},
			},
		},
	})
	assertStore(t, r, "target-2", "target", "4")
	requireActionsLength(t, r, 3)

	r.ObjectAdded(getObject(r, "target-2", "target"))
	requireActionsLength(t, r, 3)
}

func TestReplicateTo_invalid(t *testing.T) {
	r := createTestReplicator(t, ReplicatorOptions{}, "source-ns")
	source := updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-[0-9]+/target",
	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 0)

	r.NamespaceAdded(addNamespace(r, "target-1"))

	assertAction(t, r, 0, &testAction{
		Action: "install",
		Object: testObject{
			Type: "0",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-1",
				ResourceVersion: "",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "0",
				},
			},
		},
	})
	assertStore(t, r, "target-1", "target", "1")
	requireActionsLength(t, r, 1)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "!!!",
	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 1)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateToNsAnnotation: "(((",
	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 1)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-[0-9]+/target",
		ReplicateOnceAnnotation: "...",
	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 1)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-[0-9]+/target",
		annotationsPrefix + "test": "true",
	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 1)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-[0-9]+/target",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 1, &testAction{
		Action: "install",
		Object: testObject{
			Type: "6",
			Data: "6",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-1",
				ResourceVersion: "1",
				Annotations: M{
					ReplicatedFromVersionAnnotation: "6",
				},
			},
		},
	})
	assertStore(t, r, "target-1", "target", "7")
	requireActionsLength(t, r, 2)
}

func TestReplicateToFrom_scenario(t *testing.T) {
	r := createTestReplicator(t, ReplicatorOptions{
		AllowAll: true,
		Labels: M{
			"labela": "valuea",
			"labelb": "valueb",
		},
	}, "target-ns")
	updateObject(r, "data-ns", "data", M{})

	source := updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-ns/target",
		ReplicateFromAnnotation: "data-ns/data",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 0, &testAction{
		Action: "install",
		Object: testObject{
			Type: "1",
			Data: "1",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "",
				Labels: M{
					"labela": "valuea",
					"labelb": "valueb",
				},
				Annotations: M{
					ReplicatedByAnnotation: "source-ns/source",
					ReplicateFromAnnotation: "data-ns/data",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "2")
	requireActionsLength(t, r, 1)

	r.ObjectAdded(getObject(r, "target-ns", "target"))
	assertAction(t, r, 1, &testAction{
		Action: "update",
		Object: testObject{
			Type: "1",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "2",
				Labels: M{
					"labela": "valuea",
					"labelb": "valueb",
				},
				Annotations: M{
					ReplicatedByAnnotation: "source-ns/source",
					ReplicateFromAnnotation: "data-ns/data",
					ReplicatedFromVersionAnnotation: "0",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "3")
	requireActionsLength(t, r, 2)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-ns/target",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 2, &testAction{
		Action: "install",
		Object: testObject{
			Type: "4",
			Data: "4",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "3",
				Labels: M{
					"labela": "valuea",
					"labelb": "valueb",
				},
				Annotations: M{
					ReplicatedByAnnotation: "source-ns/source",
					ReplicatedFromVersionAnnotation: "4",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "5")
	requireActionsLength(t, r, 3)
}

func TestReplicateToFrom_annotations(t *testing.T) {
	r := createTestReplicator(t, ReplicatorOptions{}, "target-ns")

	source := updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-ns/target",
		ReplicateFromAnnotation: "data-ns/data",
		ReplicateOnceAnnotation: "true",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 0, &testAction{
		Action: "install",
		Object: testObject{
			Type: "0",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "",
				Annotations: M{
					ReplicatedByAnnotation: "source-ns/source",
					ReplicateFromAnnotation: "data-ns/data",
					ReplicateOnceAnnotation: "true",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "1")
	requireActionsLength(t, r, 1)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-ns/target",
		ReplicateFromAnnotation: "other-ns/other",
		ReplicateOnceAnnotation: "true",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 1, &testAction{
		Action: "install",
		Object: testObject{
			Type: "2",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "1",
				Annotations: M{
					ReplicatedByAnnotation: "source-ns/source",
					ReplicateFromAnnotation: "other-ns/other",
					ReplicateOnceAnnotation: "true",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "3")
	requireActionsLength(t, r, 2)

	source = updateObject(r, "source-ns", "source", M{
		ReplicateToAnnotation: "target-ns/target",
		ReplicateFromAnnotation: "other-ns/other",
	})
	r.ObjectAdded(source)
	assertAction(t, r, 2, &testAction{
		Action: "install",
		Object: testObject{
			Type: "4",
			Data: "0",
			Meta: metav1.ObjectMeta{
				Name: "target",
				Namespace: "target-ns",
				ResourceVersion: "3",
				Annotations: M{
					ReplicatedByAnnotation: "source-ns/source",
					ReplicateFromAnnotation: "other-ns/other",
				},
			},
		},
	})
	assertStore(t, r, "target-ns", "target", "5")
	requireActionsLength(t, r, 3)
}

func Test_newFilledInformer(t *testing.T) {
	resyncPeriod := time.Hour
	sleep := 500 * time.Millisecond

	all := []string{"ns1", "ns2", "ns3"}
	copies := map[string]*v1.Namespace{}
	todo := map[string]bool{}
	objects := []runtime.Object{}
	for index, ns := range all {
		todo[ns] = true
		object := v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
				ResourceVersion: "v" + strconv.Itoa(index),
				Labels: M{
					"l" + strconv.Itoa(index): strconv.Itoa(index),
				},
				Annotations: M{
					"a" + strconv.Itoa(index): strconv.Itoa(index),
				},
			},
		}
		copies[ns] = object.DeepCopy()
		objects = append(objects, object.DeepCopy())
	}

	var store cache.Store
	var controller cache.Controller
	nsAdded := func (object interface{}) {
		ns, ok := object.(*v1.Namespace)
		require.True(t, ok)
		assert.Truef(t, todo[ns.Name], "already added %s", ns.Name)
		assert.Equalf(t, copies[ns.Name], ns, "added different %s", ns.Name)
		delete(todo, ns.Name)
		unseen := map[string]bool{}
		for _, n := range all {
			unseen[n] = true
		}
		for _, o := range store.List() {
			n, ok := o.(*v1.Namespace)
			require.True(t, ok)
			assert.Equalf(t, copies[n.Name], n, "stored different %s", n.Name)
			delete(unseen, n.Name)
		}
		assert.Emptyf(t, unseen, "unseen when added %s", ns.Name)
	}

	var toUpdate *v1.Namespace
	nsUpdated := func (old interface{}, new interface{}) {
		ns, ok := new.(*v1.Namespace)
		require.True(t, ok)
		ons, ok := old.(*v1.Namespace)
		require.True(t, ok)
		if assert.NotNilf(t, toUpdate, "unexpected update %s", ns.Name) && assert.Equal(t, toUpdate.Name, ns.Name, "unexpected update %s, expected %s", ns.Name, toUpdate.Name) {
			assert.Equalf(t, toUpdate, ns, "updated differnt new %s", ns.Name)
			assert.Equalf(t, copies[ns.Name], ons, "updated differnt old %s", ns.Name)
			toUpdate = nil
		}
	}

	var toDelete *v1.Namespace
	nsDelete := func (object interface{}) {
		ns, ok := object.(*v1.Namespace)
		require.True(t, ok)
		if assert.NotNilf(t, toDelete, "unexpected delete %s", ns.Name) && assert.Equal(t, toDelete.Name, ns.Name, "unexpected delete %s, expected %s", ns.Name, toDelete.Name) {
			assert.Equalf(t, toDelete, ns, "deleted differnt %s", ns.Name)
			toDelete = nil
		}
	}

	client := fake.NewSimpleClientset(objects...)
	namespaces := client.CoreV1().Namespaces()
	store, controller = newFilledInformer(
		&cache.ListWatch{
			ListFunc: func(lo metav1.ListOptions) (runtime.Object, error) {
				return namespaces.List(lo)
			},
			WatchFunc: namespaces.Watch,
		},
		&v1.Namespace{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    nsAdded,
			UpdateFunc: nsUpdated,
			DeleteFunc: nsDelete,
		},
	)
	go controller.Run(wait.NeverStop)

	time.Sleep(sleep)
	assert.Emptyf(t, todo, "todo")
	toUpdate = &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ns1",
			ResourceVersion: "v" + strconv.Itoa(len(all)),
			Labels: M{
				"l" + strconv.Itoa(len(all)): strconv.Itoa(len(all)),
			},
			Annotations: M{
				"a" + strconv.Itoa(len(all)): strconv.Itoa(len(all)),
			},
		},
	}
	namespaces.Update(toUpdate.DeepCopy())
	toDelete = copies["ns2"]
	namespaces.Delete("ns2", &metav1.DeleteOptions{})
	time.Sleep(sleep)
	assert.Nil(t, toUpdate, "update expected")
	assert.Nil(t, toDelete, "delete expected")
}
