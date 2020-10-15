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

func getActions(r *objectReplicator) []*testAction {
	return r.replicatorActions.(*testActions).Actions
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

type M = map[string]string

func TestReplicateFrom_normal(t *testing.T) {
	r := createReplicator(false)
	source := updateObject(r, "source-ns", "source", M{
		ReplicationAllowedAnnotation: "true",
	})
	r.ObjectAdded(source)
	require.Equal(t, 0, len(getActions(r)), "len(actions)")
	target := updateObject(r, "target-ns", "target", M{
		ReplicateFromAnnotation: "source-ns/source",
	})
	r.ObjectAdded(target)

	actions := getActions(r)
	if len(actions) > 0 &&
			assert.Equal(t, "update", actions[0].Action, "[0].Action") &&
			assert.Equal(t, "target-ns", actions[0].Object.Meta.Namespace, "[0].Object.Meta.Namespace") &&
			assert.Equal(t, "target", actions[0].Object.Meta.Name, "[0].Object.Meta.Name") {
		assert.False(t, actions[0].Conflict, "[0].Conflict")
		assert.Equal(t, "1", actions[0].Object.Type, "[0].Object.Type")
		assert.Equal(t, "0", actions[0].Object.Data, "[0].Object.Data")
		assert.Equal(t, "1", actions[0].Object.Meta.ResourceVersion, "[0].Object.Meta.ResourceVersion")
		assert.Equal(t, "0", actions[0].Object.Meta.Annotations[ReplicatedFromVersionAnnotation], "[0].Object.Meta.Annotations[ReplicatedFromVersionAnnotation]")
	}

	require.Equal(t, 1, len(getActions(r)), "len(actions)")
	r.ObjectAdded(getObject(r, "target-ns", "target"))
	require.Equal(t, 1, len(getActions(r)), "len(actions)")
	source = updateObject(r, "source-ns", "source", nil)
	r.ObjectAdded(source)

	actions = getActions(r)
	if len(actions) > 1 &&
			assert.Equal(t, "update", actions[1].Action, "[1].Action") &&
			assert.Equal(t, "target-ns", actions[1].Object.Meta.Namespace, "[1].Object.Meta.Namespace") &&
			assert.Equal(t, "target", actions[1].Object.Meta.Name, "[1].Object.Meta.Name") {
		assert.False(t, actions[1].Conflict, "[1].Conflict")
		assert.Equal(t, "1", actions[1].Object.Type, "[1].Object.Type")
		assert.Equal(t, "3", actions[1].Object.Data, "[1].Object.Data")
		assert.Equal(t, "2", actions[1].Object.Meta.ResourceVersion, "[1].Object.Meta.ResourceVersion")
		assert.Equal(t, "3", actions[1].Object.Meta.Annotations[ReplicatedFromVersionAnnotation], "[1].Object.Meta.Annotations[ReplicatedFromVersionAnnotation]")
	}
	require.Equal(t, 2, len(actions), "len(actions)")
}
