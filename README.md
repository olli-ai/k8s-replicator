# ConfigMaps and secrets replication for Kubernetes

`k8s-replicator` is a stateless controller used to replicate configMaps and secrets, to make them available in multiple namespaces, or to create persistent objects form helm deployments, that won't be cleared by the next deployment.

This controller is designed to solve those problems:
- Secrets and configMaps are only available in a specific namespace, and there is no easy way to make a configMap or secret available across the whole cluster.
- Helm deployments systematically replace all the objects and don't allow any persistent values across successive deployment, which is especially problematic for randomly generated passwords.

## Deployment

### From helm repository

```shellsession
$ helm repo add olli-ai https://olli-ai.github.io/helm-charts/
$ helm install olli-ai/k8s-replicator --name k8s-replicator
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
  - `k8s-replicator/replicate-once`: Set it to `"true"` for being replicated only once, no matter to the future changes of the source. Can be useful if the source is a randomly generated password, but you don't want your local passowrd to change anymore.

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
  - `k8s-replicator/replication-allowed-namespaces`: a comma separated list of namespaces or namespaces patterns to explicitely allow. ex: `"my-namespace,test-namespace-[0-9]+"`

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
  - `k8s-replicator/replicate-to`: The target(s) of the annotation, comma separated. Can be a name, a full path `<namespace>/<name>`, or a pattern `<namesapce_pattern>/<name>`. If just given a name, it will be combined with the namespace of the source, or with the `k8s-replicator/replicate-to-namespaces` annotation if present. ex: `"other-secret,other-namespace/another-secret,test-namespace-[0-9]+/nyan-secret"`
  - `k8s-replicator/replicate-to-namespaces`: The target namespace(s) and namespace pattern(s) for replication, comma separated. it will be combined with the name of the source, or with the `k8s-replicator/replicate-to` if present. ex: `"other-namespace,test-namespace-[0-9]+"`

Other annotations are:
  - `k8s-replicator/replicate-once`: Set it to `"true"` for being replicated only once, no matter future changes. Can be useful if the secret is a password randomly generated by helm, and you want stable copy that won't change on future deployments.
  - `k8s-replicator/replicate-once-version`: When a different version is set, this secret or confingMap is replicated again, even if replicated once. It allows a thinner control on the `k8s-replicator/replicate-once` annotation. Can be any string.

Replication will be cancelled if the target secret or configMap already exists but was not created by replication from this source. However, as soon as that existing target is deleted, it will be replaced by a replication of the source.

Once the source secret or configMap is deleted or its annotations are changed, the target is deleted.

### Mixing both

`k8s-replicator/replicate-from` and `k8s-replicator/replicate-to` annotations can be mixed together, in order to replicate the data of another secret or configMap to a specified target. It can combine both sets of annotations, and will create a target secret or configMap that acts according to its `k8s-replicator/replicate-from` annotations.

This is especially useful because the generated secret or configMap is not managed by helm and won't be erased by helm deployments, thus avoiding replicated configmaps to be reset at each deployment, and allowing to store a randomly generated passwords.

The generated secret or configMap is delete if its creator is deleted, and cleared if its source is deleted or does not allow replication.

### Handling errors

The state of the replicated objects is stored in the annotations, so `k8s-replicator` is resilient to restarts and kubernetes errors, and won't perform redundant actions. All updates / creations / deletions are performed against the `ResourceVersion`, so any outdated update will fail.

If any annotations is detected to be illformed, no actions will be performed. This ensures that no unintended action is performed because of a human error, avoiding to unintentionally delete or clear a secret or configMap.

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

Or you can give your secret a target, such that it won't be reset by helm on further deployments

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
  name: my-tls
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
spec:
  tls:
  - hosts:
    - example.com
    secretName: my-tls
```

### Configurable secret for helm charts

Allow different possible configuration in your `values.yaml`

```yaml
admin:
  source: # another secret as admin login/password
  login: admin # the admin login, if no secret provided
  password: # the admin password, randomly generated if not provided
  version: 1.0.0 # increase if the format changes
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
    k8s-replicator/replicate-once-version: v{{ .Values.admin.version }}-{{ .Values.admin.login }}
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

This way, the source of the secret (external secret, helm value, or random) can be easily configured, the secret won't be erased by future deployment and the random password won't change unless the login changes.

## Configuration

| Helm parameter   | Argument             | Description                                                                                                            | Default                                                 |
|------------------|----------------------|------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------|
| allowAll          | --allow-all          | Implicitly allow to copy from any secret or configMap                                                                  | `false`                                                 |
| ignoreUnknown     | --ignore-unknown     | Unknown annotations with the same prefix do not raise an error                                                         | `false`                                                 |
| resyncPeriod      | --resync-period      | How often the kubernetes informers should resynchronize                                                                | `30m`                                                   |
| runReplicators    | --run-replicators    | The replicators to run, `all` or a comma-separated list of case-insensitive replicators (`secret,configMap`)           | `all`                                                   |
| annotationsPrefix | --annotations-prefix | The prefix to use on every annotations                                                                                 | `k8s-replicator`                                        |
| createWithLabels  | --create-with-labels | A comma-separated list of labels and values to apply to created secrets and configMaps (`label1=value1,label2=value2`) | `app.kubernetes.io/managed-by={.Values.prefixOverride}` |
| nameOverride      |                      | Overrides the name used in the label selector and the default name of the resources                                    | `{.Chart.Name}`                                         |
| fullnameOverride  |                      | Overrides the name of the resources                                                                                    | `{.Release.Name}-{.Values.nameOverride}`                |
|                   | --status-address     | The address for the status HTTP endpoint                                                                               | `:9102`                                                 |
|                   | --kube-config        | The path to Kubernetes config file                                                                                     |                                                         |

## Replicating more resources

`k8s-replicator` can easily be extended to replicate any resource in kubernetes, as long as it has a namespace and annotations. To create a new replicator, you need to provide:
- a constructor that will provide the watcher for the desired resource
- functions that provide the actions `update`, `clear`, `install` and `delete`
