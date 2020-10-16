package replicate

import (
	// "fmt"
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigMap_getMeta(t *testing.T) {
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
	assert.Equal(t, copy, _configMapActions.getMeta(object))
}

func TestConfigMap_update(t *testing.T) {
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
	err = _configMapActions.update(replicator, old2, source2, annotations)
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
	stored, ok, err := replicator.objectStore.GetByKey("test-ns/test-update")
	require.NoError(t, err, "store")
	require.True(t, ok, "store")
	assert.Equal(t, expected, stored.(*v1.ConfigMap), "store")
}

func TestConfigMap_clear(t *testing.T) {
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
	err = _configMapActions.clear(replicator, todo2, annotations)
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
	stored, ok, err := replicator.objectStore.GetByKey("test-ns/test-clear")
	require.NoError(t, err, "store")
	require.True(t, ok, "store")
	assert.Equal(t, expected, stored.(*v1.ConfigMap), "store")
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
	err := _configMapActions.install(replicator, meta, source2, nil)
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
	stored, ok, err := replicator.objectStore.GetByKey("test-ns/test-install")
	require.NoError(t, err, "store")
	require.True(t, ok, "store")
	assert.Equal(t, expected, stored.(*v1.ConfigMap), "store")
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
	err = _configMapActions.install(replicator, meta, source2, nil)
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
	stored, ok, err := replicator.objectStore.GetByKey("test-ns/test-install")
	require.NoError(t, err, "store")
	require.True(t, ok, "store")
	assert.Equal(t, expected, stored.(*v1.ConfigMap), "store")
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
	err := _configMapActions.install(replicator, meta, source2, copy2)
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
	stored, ok, err := replicator.objectStore.GetByKey("test-ns/test-install")
	require.NoError(t, err, "store")
	require.True(t, ok, "store")
	assert.Equal(t, expected, stored.(*v1.ConfigMap), "store")
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
	err = _configMapActions.install(replicator, meta, source2, copy2)
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
	stored, ok, err := replicator.objectStore.GetByKey("test-ns/test-install")
	require.NoError(t, err, "store")
	require.True(t, ok, "store")
	assert.Equal(t, expected, stored.(*v1.ConfigMap), "store")
}

func TestConfigMap_delete(t *testing.T) {
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
	err = _configMapActions.delete(replicator, todo2)
	require.NoError(t, err)
	assert.Equal(t, todo, todo2, "todo changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "delete", watcher.Actions[1].GetVerb())
	require.Equal(t, "test-delete", watcher.Actions[1].(DeleteAction).GetName())
	// TODO: test delete option (impossible with the current implementation)
	_, err = configmaps.Get("test-clear", metav1.GetOptions{})
	require.Error(t, err)
	_, ok, err := replicator.objectStore.GetByKey("test-ns/test-install")
	require.NoError(t, err, "store")
	assert.False(t, ok, "store")
}
