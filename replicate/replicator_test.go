package replicate

import (
	"fmt"
	"log"
	"strconv"
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	Incr    int
	Actions []*testAction
}

func hasConflict(r *replicatorProps, meta *metav1.ObjectMeta) bool {
	current, ok, err := r.objectStore.Get(&testObject{
		Meta: *meta,
	})
	if err != nil {
		panic(err.Error())
	}
	if !ok {
		return meta.ResourceVersion != ""
	}
	return meta.ResourceVersion != current.(*testObject).Meta.ResourceVersion
}

func (*testActions) getMeta(object interface{}) *metav1.ObjectMeta {
	return &object.(*testObject).Meta
}

func (a *testActions) update(r *replicatorProps, object interface{}, sourceObject interface{}, annotations map[string]string) error {
	target := object.(*testObject)
	source := sourceObject.(*testObject)
	conflict := hasConflict(r, &target.Meta)
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
	for k, v := range(annotations) {
		m[k] = v
	}
	action.Object.Meta.Annotations = m
	a.Actions = append(a.Actions, action)
	if conflict {
		log.Printf("update conflict %s/%s", target.Meta.Namespace, target.Meta.Name)
		return fmt.Errorf("conflict %s/%s", target.Meta.Namespace, target.Meta.Name)
	}
	log.Printf("updating test %s/%s", target.Meta.Namespace, target.Meta.Name)
	if err := r.objectStore.Update(action.Object.Refresh(a)); err != nil {
		panic(err.Error())
	}
	return nil
}

func (a *testActions) clear(r *replicatorProps, object interface{}, annotations map[string]string) error {
	target := object.(*testObject)
	conflict := hasConflict(r, &target.Meta)
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
	for k, v := range(annotations) {
		m[k] = v
	}
	action.Object.Meta.Annotations = m
	a.Actions = append(a.Actions, action)
	if conflict {
		log.Printf("clear conflict %s/%s", target.Meta.Namespace, target.Meta.Name)
		return fmt.Errorf("conflict %s/%s", target.Meta.Namespace, target.Meta.Name)
	}
	log.Printf("clearing test %s/%s", target.Meta.Namespace, target.Meta.Name)
	if err := r.objectStore.Update(action.Object.Refresh(a)); err != nil {
		panic(err.Error())
	}
	return nil
}

func (a *testActions) install(r *replicatorProps, meta *metav1.ObjectMeta, sourceObject interface{}, dataObject interface{}) error {
	source := sourceObject.(*testObject)
	data := ""
	if dataObject != nil {
		data = dataObject.(*testObject).Data
	}
	conflict := hasConflict(r, meta)
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
		return fmt.Errorf("conflict %s/%s", meta.Namespace, meta.Name)
	}
	log.Printf("installing test %s/%s", meta.Namespace, meta.Name)
	if err := r.objectStore.Update(action.Object.Refresh(a)); err != nil {
		panic(err.Error())
	}
	return nil
}

func (a *testActions) delete(r *replicatorProps, object interface{}) error {
	target := object.(*testObject)
	conflict := hasConflict(r, &target.Meta)
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
	if err := r.objectStore.Delete(&action.Object); err != nil {
		panic(err.Error())
	}
	return nil
}

func testKey(object interface{}) (string, error) {
	meta := object.(*testObject).Meta
	return fmt.Sprintf("%s/%s", meta.Namespace, meta.Name), nil
}

func namespaceKey(object interface{}) (string, error) {
	return object.(*v1.Namespace).Name, nil
}

func createReplicator(allowAll bool, namespaces ...string) *objectReplicator {
	replicator := &objectReplicator{
		replicatorProps: replicatorProps{
			Name:            "test",
			allowAll:        allowAll,

			targetsFrom:     make(map[string][]string),
			targetsTo:       make(map[string][]string),

			watchedTargets:  make(map[string][]string),
			watchedPatterns: make(map[string][]targetPattern),

			objectStore:     cache.NewStore(testKey),
			namespaceStore:  cache.NewStore(namespaceKey),
		},
		replicatorActions: &testActions{},
	}
	if len(namespaces) > 0 {
		objects := []interface{}{}
		for _, ns := range(namespaces) {
			objects = append(objects, &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ns,
				},
			})
		}
		if err := replicator.namespaceStore.Replace(objects, ""); err != nil {
			panic(err.Error())
		}
	}
	return replicator
}

func addNamespace(r *objectReplicator, namespace string) *v1.Namespace {
	object := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	if err := r.namespaceStore.Update(object); err != nil {
		panic(err.Error())
	}
	return object
}

func deleteNamespace(r *objectReplicator, namespace string) *v1.Namespace {
	object := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	if err := r.namespaceStore.Delete(object); err != nil {
		panic(err.Error())
	}
	return object
}

func getObject(r *objectReplicator, namespace string, name string) *testObject {
	object, ok, err := r.objectStore.GetByKey(fmt.Sprintf("%s/%s", namespace, name))
	if err != nil {
		panic(err.Error())
	}
	if !ok {
		return nil
	}
	return object.(*testObject)
}

func updateObject(r *objectReplicator, namespace string, name string, annotations map[string]string) *testObject {
	if annotations == nil {
		annotations = getObject(r, namespace, name).Meta.Annotations
	}
	copy := map[string]string{}
	for k, v := range(annotations) {
		copy[k] = v
	}
	actions := r.replicatorActions.(*testActions)
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
	if err := r.objectStore.Update(object); err != nil {
		panic(err.Error())
	}
	return object
}

func deleteObject(r *objectReplicator, namespace string, name string) *testObject {
	object := getObject(r, namespace, name)
	if err := r.objectStore.Delete(object); err != nil {
		panic(err.Error())
	}
	return object
}

func assertAction(t *testing.T, r *objectReplicator, index int, action *testAction) {
	actions := r.replicatorActions.(*testActions).Actions
	prefix := fmt.Sprintf("[%d].", index)
	if len(actions) > index &&
			assert.Equal(t, action.Action, actions[index].Action, prefix + "Action") &&
			assert.Equal(t, action.Object.Meta.Namespace, actions[index].Object.Meta.Namespace, prefix + "Object.Meta.Namespace") &&
			assert.Equal(t, action.Object.Meta.Name, actions[index].Object.Meta.Name, prefix + "Object.Meta.Name") {
		assert.Equal(t, action.Conflict, actions[index].Conflict, prefix + "Conflict")
		assert.Equal(t, action.Object.Type, actions[index].Object.Type, prefix + "Object.Type")
		assert.Equal(t, action.Object.Data, actions[index].Object.Data, prefix + "Object.Data")
		assert.Equal(t, action.Object.Meta.ResourceVersion, actions[index].Object.Meta.ResourceVersion, prefix + "Object.Meta.ResourceVersion")
		testAnnotations := actions[index].Object.Meta.Annotations
		for k, v := range(action.Object.Meta.Annotations) {
			assert.Equal(t, v, testAnnotations[k], fmt.Sprintf("[%v]Object.Meta.Annotations[\"%v\"]", index, k))
		}
	}
}

func requireActionsLength(t *testing.T, r *objectReplicator, length int) {
	actions := r.replicatorActions.(*testActions).Actions
	require.Equal(t, length, len(actions), "len(actions)")
}

type M = map[string]string

func TestReplicateFrom_normal(t *testing.T) {
	r := createReplicator(false)
	source := updateObject(r, "source-ns", "source", M{
		ReplicationAllowedAnnotation: "true",
	})
	r.ObjectAdded(source)
	requireActionsLength(t, r, 0)
	target := updateObject(r, "target-ns", "target", M{
		ReplicateFromAnnotation: "source-ns/source",
	})

	r.ObjectAdded(target)
	assertAction(t, r, 0, &testAction{
		Action: "update",
		Conflict: false,
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
	requireActionsLength(t, r, 1)

	r.ObjectAdded(getObject(r, "target-ns", "target"))
	requireActionsLength(t, r, 1)
	source = updateObject(r, "source-ns", "source", nil)

	r.ObjectAdded(source)
	assertAction(t, r, 1, &testAction{
		Action: "update",
		Conflict: false,
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
	requireActionsLength(t, r, 2)
}
