# Simple Custom Controller
In this project we create a k8s CRD (Custom Resource Defintion) and a controller for it. The controller follows the pattern shown in [kubernetes/sample-controller](https://github.com/kubernetes/sample-controller).

### The CRD (Custom Resource Definition)
```go

type Book struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BookSpec   `json:"spec"`
	Status BookStatus `json:"status,omitempty"`
}

type BookSpec struct {
	DeploymentName string           `json:"deploymentName"`
	Replicas       *int32           `json:"replicas"`
	Container      corev1.Container `json:"container"`
}

type BookStatus struct {
	AvailableReplicas int32 `json:"availableReplicas"`
}

type BookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Book `json:"items"`
}
```
And, the auto-generated (by controller-gen) yaml file can be found at [manifests/simplecustomcontroller.crd.com_books.yaml](manifests/simplecustomcontroller.crd.com_books.yaml)

### The CR (Custom Resource)
So, a valid CR for our CRD is-
```yaml
apiVersion: simplecustomcontroller.crd.com/v1
kind: Book
metadata:
  name: example-book
spec:
  deploymentName: example-book
  replicas: 1
  container:
    name: book-store
    image: shiponcs/book-store-api-server
    ports:
      - containerPort: 8080
```
### What does this Controller do?
The controller registers a `Book` type Custom Resource in k8s api-server and manage its objects.
If we create a Custom Resource of `Kind: Book`, the controller will take the following actions

- Create a deployment to create pods with `container` value given in the applied Book resource
- Create Service for the deployment
- Create a deployment to deploy Envoy with HTTP proxy configuration
- Create LoadBalancer type service for Envoy
- Take appropriate action on receiving events from api-server
- Periodically sync the current state with desired state

### Relevant
The controller deploys this- [shiponcs/golang-rest-api-server](https://github.com/shiponcs/golang-rest-api-server/).
