package replicate

import (
	"testing"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigMap_GetMeta(t *testing.T) {
	object := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test",
			Annotations: M{
				"test-annotation": "test",
			},
		},
		Data: M{
			"test": "test-data",
		},
		BinaryData: MB{
			"test": []byte("test-data"),
		},
	}
	copy := object.ObjectMeta.DeepCopy()
	assert.Equal(t, copy, _configMapActions.GetMeta(object))
}

func TestConfigMap_Update(t *testing.T) {
	replicator, watcher := createReplicator(_configMapActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	configmaps := replicator.client.CoreV1().ConfigMaps("test-ns")

	old, err := configmaps.Create(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-update",
			Annotations: M{
				"test-annotation": "old",
				"test-old": "annotation",
			},
			ResourceVersion: "old",
		},
		Data: M{
			"test-data": "old",
			"test-old": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("old"),
			"test-old": []byte("binary"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	source := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Data: M{
			"test-data": "source",
			"test-source": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("source"),
			"test-source": []byte("binary"),
		},
	}
	annotations := M{
		"test-annotation": "new",
		"test-new": "annotation",
	}

	old2 := old.DeepCopy()
	source2 := source.DeepCopy()
	store, err := _configMapActions.Update(replicator.client, old2, source2, annotations)
	require.NoError(t, err)
	assert.Equal(t, old, old2, "old changed")
	assert.Equal(t, source, source2, "source changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "update", watcher.Actions[1].GetVerb())
	sent, ok := watcher.Actions[1].(UpdateAction).GetObject().(*v1.ConfigMap)
	require.True(t, ok, "configmap")
	new, err := configmaps.Get("test-update", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-update",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "old",
		},
		Data: M{
			"test-data": "source",
			"test-source": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("source"),
			"test-source": []byte("binary"),
		},
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	new, ok = store.(*v1.ConfigMap)
	if assert.True(t, ok, "store") {
		assert.Equal(t, expected, new, "store")
	}
}

func TestConfigMap_Clear(t *testing.T) {
	replicator, watcher := createReplicator(_configMapActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	configmaps := replicator.client.CoreV1().ConfigMaps("test-ns")

	todo, err := configmaps.Create(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-clear",
			Annotations: M{
				"test-annotation": "todo",
				"test-todo": "annotation",
			},
			ResourceVersion: "todo",
		},
		Data: M{
			"test-data": "todo",
			"test-todo": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("todo"),
			"test-todo": []byte("binary"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	annotations := M{
		"test-annotation": "done",
		"test-done": "annotation",
	}

	todo2 := todo.DeepCopy()
	store, err := _configMapActions.Clear(replicator.client, todo2, annotations)
	require.NoError(t, err)
	assert.Equal(t, todo, todo2, "todo changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "update", watcher.Actions[1].GetVerb())
	sent, ok := watcher.Actions[1].(UpdateAction).GetObject().(*v1.ConfigMap)
	require.True(t, ok, "configmap")
	new, err := configmaps.Get("test-clear", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-clear",
			Annotations: M{
				"test-annotation": "done",
				"test-done": "annotation",
			},
			ResourceVersion: "todo",
		},
		Data: nil,
		BinaryData: nil,
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	new, ok = store.(*v1.ConfigMap)
	if assert.True(t, ok, "store") {
		assert.Equal(t, expected, new, "store")
	}
}

func TestConfigMap_install_create_empty(t *testing.T) {
	replicator, watcher := createReplicator(_configMapActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	configmaps := replicator.client.CoreV1().ConfigMaps("test-ns")

	source := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Data: M{
			"test-data": "source",
			"test-source": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("source"),
			"test-source": []byte("binary"),
		},
	}
	meta := &metav1.ObjectMeta{
		Namespace: "test-ns",
		Name: "test-install",
		Annotations: M{
			"test-annotation": "new",
			"test-new": "annotation",
		},
		ResourceVersion: "",
	}

	source2 := source.DeepCopy()
	store, err := _configMapActions.Install(replicator.client, meta, source2, nil)
	require.NoError(t, err)
	assert.Equal(t, source, source2, "source changed")
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")
	require.Equal(t, "create", watcher.Actions[0].GetVerb())
	sent, ok := watcher.Actions[0].(CreateAction).GetObject().(*v1.ConfigMap)
	require.True(t, ok, "configmap")
	new, err := configmaps.Get("test-install", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "",
		},
		Data: nil,
		BinaryData: nil,
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	new, ok = store.(*v1.ConfigMap)
	if assert.True(t, ok, "store") {
		assert.Equal(t, expected, new, "store")
	}
}

func TestConfigMap_install_update_empty(t *testing.T) {
	replicator, watcher := createReplicator(_configMapActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	configmaps := replicator.client.CoreV1().ConfigMaps("test-ns")

	_, err := configmaps.Create(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "old",
				"test-old": "annotation",
			},
			ResourceVersion: "old",
		},
		Data: M{
			"test-data": "old",
			"test-old": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("old"),
			"test-old": []byte("binary"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	source := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Data: M{
			"test-data": "source",
			"test-source": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("source"),
			"test-source": []byte("binary"),
		},
	}
	meta := &metav1.ObjectMeta{
		Namespace: "test-ns",
		Name: "test-install",
		Annotations: M{
			"test-annotation": "new",
			"test-new": "annotation",
		},
		ResourceVersion: "old",
	}

	source2 := source.DeepCopy()
	store, err := _configMapActions.Install(replicator.client, meta, source2, nil)
	require.NoError(t, err)
	assert.Equal(t, source, source2, "source changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "update", watcher.Actions[1].GetVerb())
	sent, ok := watcher.Actions[1].(UpdateAction).GetObject().(*v1.ConfigMap)
	require.True(t, ok, "configmap")
	new, err := configmaps.Get("test-install", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "old",
		},
		Data: nil,
		BinaryData: nil,
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	new, ok = store.(*v1.ConfigMap)
	if assert.True(t, ok, "store") {
		assert.Equal(t, expected, new, "store")
	}
}

func TestConfigMap_install_create_data(t *testing.T) {
	replicator, watcher := createReplicator(_configMapActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	configmaps := replicator.client.CoreV1().ConfigMaps("test-ns")

	source := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Data: M{
			"test-data": "source",
			"test-source": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("source"),
			"test-source": []byte("binary"),
		},
	}
	copy := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-copy",
			Annotations: M{
				"test-annotation": "copy",
				"test-copy": "annotation",
			},
			ResourceVersion: "copy",
		},
		Data: M{
			"test-data": "copy",
			"test-copy": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("copy"),
			"test-copy": []byte("binary"),
		},
	}
	meta := &metav1.ObjectMeta{
		Namespace: "test-ns",
		Name: "test-install",
		Annotations: M{
			"test-annotation": "new",
			"test-new": "annotation",
		},
		ResourceVersion: "",
	}

	source2 := source.DeepCopy()
	copy2 := copy.DeepCopy()
	store, err := _configMapActions.Install(replicator.client, meta, source2, copy2)
	require.NoError(t, err)
	assert.Equal(t, source, source2, "source changed")
	assert.Equal(t, copy, copy2, "copy changed")
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")
	require.Equal(t, "create", watcher.Actions[0].GetVerb())
	sent, ok := watcher.Actions[0].(CreateAction).GetObject().(*v1.ConfigMap)
	require.True(t, ok, "configmap")
	new, err := configmaps.Get("test-install", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "",
		},
		Data: M{
			"test-data": "copy",
			"test-copy": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("copy"),
			"test-copy": []byte("binary"),
		},
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	new, ok = store.(*v1.ConfigMap)
	if assert.True(t, ok, "store") {
		assert.Equal(t, expected, new, "store")
	}
}

func TestConfigMap_install_update_data(t *testing.T) {
	replicator, watcher := createReplicator(_configMapActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	configmaps := replicator.client.CoreV1().ConfigMaps("test-ns")

	_, err := configmaps.Create(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "old",
				"test-old": "annotation",
			},
			ResourceVersion: "old",
		},
		Data: M{
			"test-data": "old",
			"test-old": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("old"),
			"test-old": []byte("binary"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	source := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Data: M{
			"test-data": "source",
			"test-source": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("source"),
			"test-source": []byte("binary"),
		},
	}
	copy := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-copy",
			Annotations: M{
				"test-annotation": "copy",
				"test-copy": "annotation",
			},
			ResourceVersion: "copy",
		},
		Data: M{
			"test-data": "copy",
			"test-copy": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("copy"),
			"test-copy": []byte("binary"),
		},
	}
	meta := &metav1.ObjectMeta{
		Namespace: "test-ns",
		Name: "test-install",
		Annotations: M{
			"test-annotation": "new",
			"test-new": "annotation",
		},
		ResourceVersion: "old",
	}

	source2 := source.DeepCopy()
	copy2 := copy.DeepCopy()
	store, err := _configMapActions.Install(replicator.client, meta, source2, copy2)
	require.NoError(t, err)
	assert.Equal(t, source, source2, "source changed")
	assert.Equal(t, copy, copy2, "copy changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "update", watcher.Actions[1].GetVerb())
	sent, ok := watcher.Actions[1].(UpdateAction).GetObject().(*v1.ConfigMap)
	require.True(t, ok, "configmap")
	new, err := configmaps.Get("test-install", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "old",
		},
		Data: M{
			"test-data": "copy",
			"test-copy": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("copy"),
			"test-copy": []byte("binary"),
		},
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	new, ok = store.(*v1.ConfigMap)
	if assert.True(t, ok, "store") {
		assert.Equal(t, expected, new, "store")
	}
}

func TestConfigMap_Delete(t *testing.T) {
	replicator, watcher := createReplicator(_configMapActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	configmaps := replicator.client.CoreV1().ConfigMaps("test-ns")

	todo, err := configmaps.Create(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-delete",
			Annotations: M{
				"test-annotation": "todo",
				"test-todo": "annotation",
			},
			ResourceVersion: "todo",
		},
		Data: M{
			"test-data": "todo",
			"test-todo": "data",
		},
		BinaryData: MB{
			"test-binary-data": []byte("todo"),
			"test-todo": []byte("binary"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	todo2 := todo.DeepCopy()
	err = _configMapActions.Delete(replicator.client, todo2)
	require.NoError(t, err)
	assert.Equal(t, todo, todo2, "todo changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "delete", watcher.Actions[1].GetVerb())
	require.Equal(t, "test-delete", watcher.Actions[1].(DeleteAction).GetName())
	// TODO: test delete option (impossible with the current implementation)
	_, err = configmaps.Get("test-clear", metav1.GetOptions{})
	require.Error(t, err)
}

func TestNewConfigMapReplicator(t *testing.T) {
	hour, err := time.ParseDuration("1h")
	require.NoError(t, err)
	second, err := time.ParseDuration("0.5s")
	require.NoError(t, err)
	client := fake.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "source-ns",
			Name: "source",
		},
		Data: M{
			"data": "source",
		},
	}, &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "target-1",
		},
	})
	replicator := NewConfigMapReplicator(client, ReplicatorOptions{AllowAll: true}, hour)
	replicator.Start()
	_, err = client.CoreV1().ConfigMaps("from-ns").Create(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "from-ns",
			Name: "from",
			Annotations: M{
				ReplicateFromAnnotation: "source-ns/source",
			},
		},
	})
	require.NoError(t, err, "from-ns/from")
	_, err = client.CoreV1().ConfigMaps("to-ns").Create(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "to-ns",
			Name: "to",
			Annotations: M{
				ReplicateToAnnotation: "target-[0-9]+/target",
				ReplicateFromAnnotation: "source-ns/source",
			},
		},
	})
	require.NoError(t, err, "to-ns/to")
	_, err = client.CoreV1().Namespaces().Create(&v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "target-2",
		},
	})
	require.NoError(t, err, "target-2")
	time.Sleep(second)

	configmap, err := client.CoreV1().ConfigMaps("from-ns").Get("from", metav1.GetOptions{})
	if assert.NoError(t, err, "from-ns/from") {
		assert.Equal(t, "source", configmap.Data["data"], "from-ns/from")
	}
	configmap, err = client.CoreV1().ConfigMaps("target-1").Get("target", metav1.GetOptions{})
	if assert.NoError(t, err, "target-1/target") {
		assert.Equal(t, "source", configmap.Data["data"], "target-1/target")
	}
	configmap, err = client.CoreV1().ConfigMaps("target-2").Get("target", metav1.GetOptions{})
	if assert.NoError(t, err, "target-2/target") {
		assert.Equal(t, "source", configmap.Data["data"], "target-2/target")
	}

	err = client.CoreV1().ConfigMaps("to-ns").Delete("to", &metav1.DeleteOptions{})
	require.NoError(t, err, "to-ns/to")
	time.Sleep(second)
	configmap, err = client.CoreV1().ConfigMaps("target-1").Get("target", metav1.GetOptions{})
	assert.Error(t, err, "target-1/target")
	configmap, err = client.CoreV1().ConfigMaps("target-2").Get("target", metav1.GetOptions{})
	assert.Error(t, err, "target-2/target")
}
