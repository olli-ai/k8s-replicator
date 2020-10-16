package replicate

import (
	// "fmt"
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecret_getMeta(t *testing.T) {
	object := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test",
			Annotations: M{
				"test-annotation": "test",
			},
		},
		Type: "test-type",
		Data: MB{
			"test": []byte("test-data"),
		},
	}
	copy := object.ObjectMeta.DeepCopy()
	assert.Equal(t, copy, _secretActions.getMeta(object))
}

func TestSecret_update(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	old, err := secrets.Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-update",
			Annotations: M{
				"test-annotation": "old",
				"test-old": "annotation",
			},
			ResourceVersion: "old",
		},
		Type: "old",
		Data: MB{
			"test-data": []byte("old"),
			"test-old": []byte("data"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	source := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Type: "data",
		Data: MB{
			"test-data": []byte("source"),
			"test-source": []byte("data"),
		},
	}
	annotations := M{
		"test-annotation": "new",
		"test-new": "annotation",
	}

	old2 := old.DeepCopy()
	source2 := source.DeepCopy()
	err = _secretActions.update(replicator, old2, source2, annotations)
	require.NoError(t, err)
	assert.Equal(t, old, old2, "old changed")
	assert.Equal(t, source, source2, "source changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "update", watcher.Actions[1].GetVerb())
	sent, ok := watcher.Actions[1].(UpdateAction).GetObject().(*v1.Secret)
	require.True(t, ok, "secret")
	new, err := secrets.Get("test-update", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-update",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "old",
		},
		Type: "old",
		Data: MB{
			"test-data": []byte("source"),
			"test-source": []byte("data"),
		},
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	stored, ok, err := replicator.objectStore.GetByKey("test-ns/test-update")
	require.NoError(t, err, "store")
	require.True(t, ok, "store")
	assert.Equal(t, expected, stored.(*v1.Secret), "store")
}

func TestSecret_clear(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	todo, err := secrets.Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-clear",
			Annotations: M{
				"test-annotation": "todo",
				"test-todo": "annotation",
			},
			ResourceVersion: "todo",
		},
		Type: "todo",
		Data: MB{
			"test-data": []byte("todo"),
			"test-todo": []byte("data"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	annotations := M{
		"test-annotation": "done",
		"test-done": "annotation",
	}

	todo2 := todo.DeepCopy()
	err = _secretActions.clear(replicator, todo2, annotations)
	require.NoError(t, err)
	assert.Equal(t, todo, todo2, "todo changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "update", watcher.Actions[1].GetVerb())
	sent, ok := watcher.Actions[1].(UpdateAction).GetObject().(*v1.Secret)
	require.True(t, ok, "secret")
	new, err := secrets.Get("test-clear", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-clear",
			Annotations: M{
				"test-annotation": "done",
				"test-done": "annotation",
			},
			ResourceVersion: "todo",
		},
		Type: "todo",
		Data: nil,
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	stored, ok, err := replicator.objectStore.GetByKey("test-ns/test-clear")
	require.NoError(t, err, "store")
	require.True(t, ok, "store")
	assert.Equal(t, expected, stored.(*v1.Secret), "store")
}

func TestSecret_install_create_empty(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	source := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Type: "source",
		Data: MB{
			"test-data": []byte("source"),
			"test-source": []byte("data"),
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
	err := _secretActions.install(replicator, meta, source2, nil)
	require.NoError(t, err)
	assert.Equal(t, source, source2, "source changed")
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")
	require.Equal(t, "create", watcher.Actions[0].GetVerb())
	sent, ok := watcher.Actions[0].(CreateAction).GetObject().(*v1.Secret)
	require.True(t, ok, "secret")
	new, err := secrets.Get("test-install", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "",
		},
		Type: "source",
		Data: nil,
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	stored, ok, err := replicator.objectStore.GetByKey("test-ns/test-install")
	require.NoError(t, err, "store")
	require.True(t, ok, "store")
	assert.Equal(t, expected, stored.(*v1.Secret), "store")
}

func TestSecret_install_update_empty(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	_, err := secrets.Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "old",
				"test-old": "annotation",
			},
			ResourceVersion: "old",
		},
		Type: "old",
		Data: MB{
			"test-data": []byte("old"),
			"test-old": []byte("data"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	source := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Type: "source",
		Data: MB{
			"test-data": []byte("source"),
			"test-source": []byte("data"),
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
	err = _secretActions.install(replicator, meta, source2, nil)
	require.NoError(t, err)
	assert.Equal(t, source, source2, "source changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "update", watcher.Actions[1].GetVerb())
	sent, ok := watcher.Actions[1].(UpdateAction).GetObject().(*v1.Secret)
	require.True(t, ok, "secret")
	new, err := secrets.Get("test-install", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "old",
		},
		Type: "source",
		Data: nil,
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	stored, ok, err := replicator.objectStore.GetByKey("test-ns/test-install")
	require.NoError(t, err, "store")
	require.True(t, ok, "store")
	assert.Equal(t, expected, stored.(*v1.Secret), "store")
}

func TestSecret_install_create_data(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	source := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Type: "source",
		Data: MB{
			"test-data": []byte("source"),
			"test-source": []byte("data"),
		},
	}
	copy := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-copy",
			Annotations: M{
				"test-annotation": "copy",
				"test-copy": "annotation",
			},
			ResourceVersion: "copy",
		},
		Type: "copy",
		Data: MB{
			"test-data": []byte("copy"),
			"test-copy": []byte("data"),
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
	err := _secretActions.install(replicator, meta, source2, copy2)
	require.NoError(t, err)
	assert.Equal(t, source, source2, "source changed")
	assert.Equal(t, copy, copy2, "copy changed")
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")
	require.Equal(t, "create", watcher.Actions[0].GetVerb())
	sent, ok := watcher.Actions[0].(CreateAction).GetObject().(*v1.Secret)
	require.True(t, ok, "secret")
	new, err := secrets.Get("test-install", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "",
		},
		Type: "source",
		Data: MB{
			"test-data": []byte("copy"),
			"test-copy": []byte("data"),
		},
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	stored, ok, err := replicator.objectStore.GetByKey("test-ns/test-install")
	require.NoError(t, err, "store")
	require.True(t, ok, "store")
	assert.Equal(t, expected, stored.(*v1.Secret), "store")
}

func TestSecret_install_update_data(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	_, err := secrets.Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "old",
				"test-old": "annotation",
			},
			ResourceVersion: "old",
		},
		Type: "old",
		Data: MB{
			"test-data": []byte("old"),
			"test-old": []byte("data"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	source := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Type: "source",
		Data: MB{
			"test-data": []byte("source"),
			"test-source": []byte("data"),
		},
	}
	copy := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-copy",
			Annotations: M{
				"test-annotation": "copy",
				"test-copy": "annotation",
			},
			ResourceVersion: "copy",
		},
		Type: "copy",
		Data: MB{
			"test-data": []byte("copy"),
			"test-copy": []byte("data"),
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
	err = _secretActions.install(replicator, meta, source2, copy2)
	require.NoError(t, err)
	assert.Equal(t, source, source2, "source changed")
	assert.Equal(t, copy, copy2, "copy changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "update", watcher.Actions[1].GetVerb())
	sent, ok := watcher.Actions[1].(UpdateAction).GetObject().(*v1.Secret)
	require.True(t, ok, "secret")
	new, err := secrets.Get("test-install", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "old",
		},
		Type: "source",
		Data: MB{
			"test-data": []byte("copy"),
			"test-copy": []byte("data"),
		},
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	stored, ok, err := replicator.objectStore.GetByKey("test-ns/test-install")
	require.NoError(t, err, "store")
	require.True(t, ok, "store")
	assert.Equal(t, expected, stored.(*v1.Secret), "store")
}

func TestSecret_delete(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	todo, err := secrets.Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-delete",
			Annotations: M{
				"test-annotation": "todo",
				"test-todo": "annotation",
			},
			ResourceVersion: "todo",
		},
		Type: "todo",
		Data: MB{
			"test-data": []byte("todo"),
			"test-todo": []byte("data"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	todo2 := todo.DeepCopy()
	err = _secretActions.delete(replicator, todo2)
	require.NoError(t, err)
	assert.Equal(t, todo, todo2, "todo changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "delete", watcher.Actions[1].GetVerb())
	require.Equal(t, "test-delete", watcher.Actions[1].(DeleteAction).GetName())
	// TODO: test delete option (impossible with the current implementation)
	_, err = secrets.Get("test-clear", metav1.GetOptions{})
	require.Error(t, err)
	_, ok, err := replicator.objectStore.GetByKey("test-ns/test-install")
	require.NoError(t, err, "store")
	assert.False(t, ok, "store")
}
