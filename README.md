# ConfigMaps and secrets replication for Kubernetes

`k8s-replicator` is a stateless controller used to replicate configMaps and secrets, to make them available in multiple namespaces, or to create persistent configMaps and secrets that won't be cleared by the next helm release.

This controller is designed to solve those problems:
- Secrets and configMaps are only available in a specific namespace, and there is no easy way to make a configMap or secret available across the whole cluster.
- Helm releases systematically replace all the objects and don't allow any persistent values across successive releases, which is especially problematic for randomly generated passwords.

## Deployment

### From helm repository

```shellsession
$ helm repo add olli-ai https://olli-ai.github.io/helm-charts/
$ helm upgrade --install k8s-replicator olli-ai/k8s-replicator
```

### Using Helm

```shellsession
$ helm upgrade --install k8s-replicator ./deploy/helm-chart/k8s-replicator
```

### Manual

```shellsession
$ # Create roles and service accounts
$ kubectl apply -f https://raw.githubusercontent.com/olli-ai/k8s-replicator/master/deploy/rbac.yaml
$ # Create actual deployment
$ kubectl apply -f https://raw.githubusercontent.com/olli-ai/k8s-replicator/master/deploy/deployment.yaml
```

## Usage

### Receiving a copy of secret or configMap

You can configure a secret or a configMap to receive a copy of another secret or configMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  annotations:
    k8s-replicator/replicate-from: default/some-secret
data: {}
```

Annotations are:
  - `k8s-replicator/replicate-from`: The source of the data to receive a copy from. Can be a full path `<namespace>/<name>`, or just a name if the source is in the same namespace.
  - `k8s-replicator/replicate-once`: Set it to `"true"` for being replicated only once, no matter to the future changes of the source. Can be useful if the source is a randomly generated password, but you don't want your local password to change anymore.

Unless you run k8s-replicator with the `--allow-all` flag, you need to explicitely allow the source to be replicated:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  annotations:
    k8s-replicator/replication-allowed: "true"
data: {}
```

At leat one of the two annotations is required (if `--allow-all` is not used):
  - `k8s-replicator/replication-allowed`: Set it to `"true"` to explicitely allow replication, or `"false"` to explicitely diswallow it
  - `k8s-replicator/replication-allowed-namespaces`: a comma separated list of namespaces or namespace patterns to explicitely allow. ex: `"my-namespace,test-namespace-[0-9]+"`

Other annotations are:
  - `k8s-replicator/replicate-once`: Set it to `"true"` for being replicated only once, no matter future changes. Can be useful if the secret is a randomly generated password, but you don't want the local copies to change anymore.
  - `k8s-replicator/replicate-once-version`: When a different version is set, this secret or confingMap is replicated again, even if replicated once. It allows a thinner control on the `k8s-replicator/replicate-once` annotation. Can be any string.

The content of the target secret of configMap will be cleared if the source does not exist, does not allow replication, or is deleted.

### Replicating a secret or configMap to other locations

You can configure a secret or a configMap to replicate itself automatically to desired locations:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  annotations:
    k8s-replicator/replicate-to: default/other-secret
data: {}
```

At leat one of the two annotations is required:
  - `k8s-replicator/replicate-to`: The target(s) of the annotation, comma separated. Can be a name `<name>`, a full path `<namespace>/<name>`, or a pattern `<namespace_pattern>/<name>`. If just given a name, it will be combined with the namespace of the source, or with the `k8s-replicator/replicate-to-namespaces` annotation if present. ex: `"other-secret,other-namespace/another-secret,test-namespace-[0-9]+/nyan-secret"`
  - `k8s-replicator/replicate-to-namespaces`: The target namespace(s) and namespace pattern(s) for replication, comma separated. it will be combined with the name of the source, or with the `k8s-replicator/replicate-to` if present. ex: `"other-namespace,test-namespace-[0-9]+"`

Other annotations are:
  - `k8s-replicator/replicate-once`: Set it to `"true"` for being replicated only once, no matter future changes. Can be useful if the secret is a password randomly generated by helm, and you want stable copy that won't change on future helm releases.
  - `k8s-replicator/replicate-once-version`: When a different version is set, this secret or confingMap is replicated again, even if replicated once. It allows a thinner control on the `k8s-replicator/replicate-once` annotation. Can be any string.

The labels given to any created target secret or configMap can be configured with the `--create-with-labels`. Replication will be cancelled if the target secret or configMap already exists but was not created by replication from this source. However, as soon as that existing target is deleted, it will be replaced by a replication of the source. As soon as any target namespace is created, required target secrets and configMaps are created.

Once the source secret or configMap is deleted or its annotations are changed, the target is deleted.

### Combining both

`k8s-replicator/replicate-from` and `k8s-replicator/replicate-to` annotations can be combined together, in order to replicate the data of another secret or configMap to a specified target. It can combine both sets of annotations, and will create a target secret or configMap that acts according to its `k8s-replicator/replicate-from` annotations.

This is especially useful because the generated secret or configMap is not managed by helm and won't be erased by helm releases, thus avoiding the secret or configMap to be shortly reset at each release, which may trigger the pods to restart if a [reloader](https://github.com/stakater/Reloader) is used.

The generated secret or configMap is deleted if its creator is deleted, and cleared if its source is deleted or does not allow replication.

### Handling errors

The state of the replicated secrets and configMaps and is stored in their annotations, so `k8s-replicator` is resilient to restarts and kubernetes errors, and won't perform redundant actions. `--resync-period` configures how often the list of resources is reloaded, which forces the replicator to check the state of the cluster. All updates / creations / deletions are performed against the `ResourceVersion`, so any outdated update will fail.

If any annotation is detected to be illformed, no action will be performed. This is also the case if an unknown annotation with the same prefix is detected, unless `--ignore-unknown` option is passed. This ensures that no unintended action is performed because of a human error, avoiding to unintentionally delete or clear a secret or configMap.

The logs of the `k8s-replicator` pod will show the full history of actions, and explanations why some of these actions are cancelled.

## Examples

### Import database credentials anywhere

Create the source secret

```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: database-credentials
  namespace: default
  annotations:
    k8s-replicator/replication-allowed: "true"
stringData:
  host: mydb.com
  database: mydb
  password: qwerty
```

You can now create an empty secret everywhere you needs this (including in helm charts)

```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: local-database-credentials
  annotations:
    k8s-replicator/replicate-from: "default/database-credentials"
```

Or you can give your secret a target, such that it won't be reset by helm on further helm releases

```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: source-database-credentials
  annotations:
    k8s-replicator/replicate-from: "default/database-credentials"
    k8s-replicator/replicate-to: "target-database-credentials"
```

### Use random password generated by an helm chart

Create your source secret with a random password, and replicate it once

```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: admin-password-source
  annotations:
    k8s-replicator/replicate-to: "admin-password"
    k8s-replicator/replicate-once: "true"
stringData:
  password: {{ randAlphaNum 64 | quote }}
```

And use it in your deployment

```yaml
apiVersion: extensions/v1beta1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: my-container
        image: gcr.io/my-project/my-container:latest
        env:
        - name: ADMIN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: admin-password
              key: password
```

### Spread your TLS key

Create your TLS secret

```yaml
apiVersion: v1
kind: Secret
type: kubernetes.io/tls
metadata:
  name: tls-example-com
  namespace: jx
  annotations:
    k8s-replicator/replicate-to-namespaces: "jx-.*"
stringData:
  tls.crt: |
    -----BEGIN CERTIFICATE-----
    [...]
    -----END CERTIFICATE-----
  tls.key: |
    -----BEGIN RSA PRIVATE KEY-----
    [...]
    -----END RSA PRIVATE KEY-----
```

And use it in your ingresses in any namespace you replicated to

```yaml
apiVersion: networking.k8s.io/v1beta1
kind: Ingress
metadata:
  name: my-ingress
  namespace: jx-production
spec:
  tls:
  - hosts:
    - subdomain.example.com
    secretName: tls-example-com
```

### Configurable secret for helm charts

Allow different possible configuration in your `values.yaml`

```yaml
admin:
  source: # another secret as admin login/password
  login: admin # the admin login, if no secret provided
  password: # the admin password, randomly generated if not provided
  version: 0 # increase this if the format changes
```

Now a `template/secret-admin.yaml` can be configured

```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: my-app-admin-source
  annotations:
    k8s-replicator/replicate-to: my-app-admin
{{- if .Values.admin.source }}
    k8s-replicator/replicate-from: {{ .Values.admin.source }}
{{- else if not .Values.admin.password }}
    k8s-replicator/replicate-once: "true"
    k8s-replicator/replicate-once-version: v{{ .Values.admin.version }}:login={{ .Values.admin.login }}
{{- end }}
{{- if not .Values.admin.source }}
stringData:
  login: {{ .Values.admin.login }}
  {{- if .Values.admin.password }}
  password: {{ .Values.admin.password | quote }}
  {{- else }}
  password: {{ randAlphaNum 64 | quote }}
  {{- end }}
{{- end }}
```

This way, the source of the secret (external secret, helm value, or random) can be easily configured, the secret won't be erased by future helm releases and the random password won't change unless the login changes.

## Configuration

| Helm parameter           | Argument               | Description                                                                                                            | Default                                                    |
|--------------------------|------------------------|------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------|
| `allowAll`               | `--allow-all`          | Implicitly allow to copy from any secret or configMap                                                                  | `false`                                                    |
| `ignoreUnknown`          | `--ignore-unknown`     | Unknown annotations with the same prefix do not raise an error                                                         | `false`                                                    |
| `resyncPeriod`           | `--resync-period`      | How often the kubernetes informers should resynchronize                                                                | `30m`                                                      |
| `runReplicators`         | `--run-replicators`    | The replicators to run, `all` or a comma-separated list of case-insensitive replicators (`secret,configMap,endpoints`) | `all`                                                      |
| `annotationsPrefix`      | `--annotations-prefix` | The prefix to use on every annotations                                                                                 | `k8s-replicator`                                           |
| `createWithLabels`       | `--create-with-labels` | A comma-separated list of labels and values to apply to created secrets and configMaps (`label1=value1,label2=value2`) | `app.kubernetes.io/managed-by={.Values.annotationsPrefix}` |
|                          | `--status-address`     | The address for the status HTTP endpoint                                                                               | `:9102`                                                    |
|                          | `--kube-config`        | The path to Kubernetes config file                                                                                     | cluster config                                             |
| `image.repository`       |                        | Provisioner image                                                                                                      | `olliai/glusterfs-client-provisioner`                      |
| `image.tag`              |                        | Version of provisioner image                                                                                           | Chart's version                                            |
| `image.pullPolicy`       |                        | Image pull policy                                                                                                      | `IfNotPresent`                                             |
| `nameOverride`           |                        | Overrides the name used in the label selector and the default name of the resources                                    | `{.Chart.Name}`                                            |
| `fullnameOverride`       |                        | Overrides the name of the resources                                                                                    | `{.Release.Name}-{.Values.nameOverride}`                   |
| `serviceAccount.create`  |                        | Creates a service account with necessary roles                                                                         | `true`                                                     |
| `serviceAccount.name`    |                        | Name of an existing service account to use                                                                             |                                                            |
| `deployment.annotations` |                        | Annotations for the deployment                                                                                         | `{}`                                                       |
| `pod.annotations`        |                        | Annotations for the pod                                                                                                | `{}`                                                       |

You can pass several replicators using `--set runReplicators='{configMap,secret}'`

## Replicating more resources

`k8s-replicator` can easily be extended to replicate any resource in kubernetes:
```golang
package mypackage

import (
    "log"
    "time"

    "github.com/olli-ai/k8s-replicator/replicate"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/cache"
)

var _myActions *myActions = &myActions{}

func NewMyReplicator(client kubernetes.Interface, options replicate.ReplicatorOptions, resyncPeriod time.Duration) replicate.Replicator {
    repl := replicate.ObjectReplicator{
        ReplicatorProps:   replicate.NewReplicatorProps(client, "myResource", options),
        ReplicatorActions: _myActions,
    }
    myResurces := MyResources(client, "")
    listWatch := cache.ListWatch{
        ListFunc: func(lo metav1.ListOptions) (runtime.Object, error) {
            return myResurces.List(lo)
        },
        WatchFunc: myResurces.Watch,
    }
    repl.InitStores(listWatch, &MyResource{}, resyncPeriod)
    return &repl
}

type myActions struct {}

func (*myActions) GetMeta(object interface{}) *metav1.ObjectMeta {
    return &object.(*MyResources).ObjectMeta
}

func (*myActions) Update(client kubernetes.Interface, object interface{}, sourceObject interface{}, annotations map[string]string) (interface{}, error) {
    mySource := sourceObject.(*MyResource)
    myObject := object.(*MyResource).DeepCopy()
    myObject.Annotations = annotations

    // TODO: copy the data from mySource to myObject

    log.Printf("updating myResource %s/%s", myObject.Namespace, myObject.Name)
    update, err := MyResources(client, myObject.Namespace).Update(myObject)
    if err != nil {
        log.Printf("error while updating myResource %s/%s: %s", myObject.Namespace, myObject.Name, err)
    }
    return update, err
}

func (*myActions) Clear(client kubernetes.Interface, object interface{}, annotations map[string]string) (interface{}, error) {
    myObject := object.(*MyResource).DeepCopy()
    myObject.Annotations = annotations

    // TODO: clear the data from myObject

    log.Printf("clearing myResource %s/%s", myObject.Namespace, myObject.Name)
    update, err := MyResources(client, myObject.Namespace).Update(myObject)
    if err != nil {
        log.Printf("error while clearing myResource %s/%s", myObject.Namespace, myObject.Name)
    }
    return update, err
}

func (*myActions) Install(client kubernetes.Interface, meta *metav1.ObjectMeta, sourceObject interface{}, dataObject interface{}) (interface{}, error) {
    // mySource := sourceObject.(*MyResource)
    myObject = MyResource{
        ObjectMeta: *meta,
    }

    // TODO: copy other meta-fields from mySource to myObject

    if dataObject != nil {
        myData := dataObject.(*MyResource)

        /// TODO: copy the data from myData to myObject

    }
    log.Printf("installing myResource %s/%s", myObject.Namespace, myObject.Name)
    var update *MyResource
    var err error
    if myObject.ResourceVersion == "" {
        update, err = MyResources(client, myObject.Namespace).Create(&myObject)
    } else {
        update, err = MyResources(client, myObject.Namespace).Update(&myObject)
    }
    if err != nil {
        log.Printf("error while installing myResource %s/%s: %s", myObject.Namespace, myObject.Name, err)
    }
    return update, err
}

func (*myActions) Delete(client kubernetes.Interface, object interface{}) error {
    myObject := object.(*MyResource)
    log.Printf("deleting myResource %s/%s", myObject.Namespace, myObject.Name)
    options := metav1.DeleteOptions{
        Preconditions: &metav1.Preconditions{
            ResourceVersion: &myObject.ResourceVersion,
        },
    }
    err := MyResources(client, myObject.Namespace).Delete(myObject.Name, &options)
    if err != nil {
        log.Printf("error while deleting myResource %s/%s: %s", myObject.Namespace, myObject.Name, err)
    }
    return err
}
```
And add the replicator function in `main.go`
```golang
var newReplicatorFuncs map[string]newReplicatorFunc = map[string]newReplicatorFunc{
    // [...]
    "myResource": mypackage.NewMyReplicator,
}
```
