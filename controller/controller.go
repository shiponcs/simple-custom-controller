package controller

import (
	"context"
	"fmt"
	bookv1 "github.com/shiponcs/simple-custom-controller/pkg/apis/simplecustomcontroller/v1"
	clientset "github.com/shiponcs/simple-custom-controller/pkg/generated/clientset/versioned"
	samplescheme "github.com/shiponcs/simple-custom-controller/pkg/generated/clientset/versioned/scheme"
	informers "github.com/shiponcs/simple-custom-controller/pkg/generated/informers/externalversions/simplecustomcontroller/v1"
	listers "github.com/shiponcs/simple-custom-controller/pkg/generated/listers/simplecustomcontroller/v1"
	"golang.org/x/time/rate"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"os"
	"time"
)

const controllerAgentName = "simple-custom-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a Book is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a book fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by book"
	// MessageResourceSynced is the message used for an Event fired when a book
	// is synced successfully
	MessageResourceSynced = "book synced successfully"
	// FieldManager distinguishes this controller from other things writing to API objects
	FieldManager = controllerAgentName
)

type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// sampleclientset is a clientset for our own API group
	sampleclientset clientset.Interface

	deploymentsLister appslisters.DeploymentLister
	deploymentsSynced cache.InformerSynced
	bookLister        listers.BookLister
	bookSynced        cache.InformerSynced
	serviceLister     corelisters.ServiceLister
	serviceSynced     cache.InformerSynced
	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.TypedRateLimitingInterface[cache.ObjectName]
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new controller
func NewController(
	ctx context.Context,
	kubeclientset kubernetes.Interface,
	Bookclientset clientset.Interface,
	deploymentInformer appsinformers.DeploymentInformer,
	serviceInformer coreinformer.ServiceInformer,
	BookInformer informers.BookInformer) *Controller {
	logger := klog.FromContext(ctx)

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
	logger.V(4).Info("Creating event broadcaster")

	eventBroadcaster := record.NewBroadcaster(record.WithContext(ctx))
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})
	ratelimiter := workqueue.NewTypedMaxOfRateLimiter(
		workqueue.NewTypedItemExponentialFailureRateLimiter[cache.ObjectName](5*time.Millisecond, 1000*time.Second),
		&workqueue.TypedBucketRateLimiter[cache.ObjectName]{Limiter: rate.NewLimiter(rate.Limit(50), 300)},
	)

	controller := &Controller{
		kubeclientset:     kubeclientset,
		sampleclientset:   Bookclientset,
		deploymentsLister: deploymentInformer.Lister(),
		deploymentsSynced: deploymentInformer.Informer().HasSynced,
		bookLister:        BookInformer.Lister(),
		bookSynced:        BookInformer.Informer().HasSynced,
		serviceLister:     serviceInformer.Lister(),
		serviceSynced:     serviceInformer.Informer().HasSynced,
		workqueue:         workqueue.NewTypedRateLimitingQueue(ratelimiter),
		recorder:          recorder,
	}

	logger.Info("Setting up event handlers")
	// Set up an event handler for when book resources change
	BookInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueBook,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueBook(new)
		},
	})
	// Set up an event handler for when Deployment resources change. This
	// handler will lookup the owner of the given Deployment, and if it is
	// owned by a book resource then the handler will enqueue that book resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Deployment resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*appsv1.Deployment)
			oldDepl := old.(*appsv1.Deployment)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})
	// setup event handler for service just like how we set for Deployment
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newService := new.(*corev1.Service)
			oldService := old.(*corev1.Service)
			if newService.ResourceVersion == oldService.ResourceVersion {
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()
	logger := klog.FromContext(ctx)

	// Start the informer factories to begin populating the informer caches
	logger.Info("Starting book controller")

	// Wait for the caches to be synced before starting workers
	logger.Info("Waiting for informer caches to sync")

	if ok := cache.WaitForCacheSync(ctx.Done(), c.deploymentsSynced, c.bookSynced, c.serviceSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	logger.Info("Starting workers", "count", workers)
	// Launch two workers to process book resources
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	logger.Info("Started workers")
	<-ctx.Done()
	logger.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	objRef, shutdown := c.workqueue.Get()
	logger := klog.FromContext(ctx)

	if shutdown {
		return false
	}

	// We call Done at the end of this func so the workqueue knows we have
	// finished processing this item. We also must remember to call Forget
	// if we do not want this work item being re-queued. For example, we do
	// not call Forget if a transient error occurs, instead the item is
	// put back on the workqueue and attempted again after a back-off
	// period.
	defer c.workqueue.Done(objRef)

	// Run the syncHandler, passing it the structured reference to the object to be synced.
	err := c.syncHandler(ctx, objRef)
	if err == nil {
		// If no error occurs then we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(objRef)
		logger.Info("Successfully synced", "objectName", objRef)
		return true
	}
	// there was a failure so be sure to report it.  This method allows for
	// pluggable error handling which can be used for things like
	// cluster-monitoring.
	utilruntime.HandleErrorWithContext(ctx, err, "Error syncing; requeuing for later retry", "objectReference", objRef)
	// since we failed, we should requeue the item to work on later.  This
	// method will add a backoff to avoid hotlooping on particular items
	// (they're probably still not going to work right away) and overall
	// controller protection (everything I've done is broken, this controller
	// needs to calm down or it can starve other useful work) cases.
	c.workqueue.AddRateLimited(objRef)
	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the book resource
// with the current status of the resource.
func (c *Controller) syncHandler(ctx context.Context, objectRef cache.ObjectName) error {
	logger := klog.LoggerWithValues(klog.FromContext(ctx), "objectRef", objectRef)

	// Get the book resource with this namespace/name
	book, err := c.bookLister.Books(objectRef.Namespace).Get(objectRef.Name)
	if err != nil {
		// The book resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleErrorWithContext(ctx, err, "Book referenced by item in work queue no longer exists", "objectReference", objectRef)
			return nil
		}

		return err
	}

	deploymentName := book.Spec.DeploymentName
	if deploymentName == "" {
		// We choose to absorb the error here as the worker would requeue the
		// resource otherwise. Instead, the next time the resource is updated
		// the resource will be queued again.
		utilruntime.HandleErrorWithContext(ctx, nil, "Deployment name missing from object reference", "objectReference", objectRef)
		return nil
	}

	// Get the deployment with the name specified in book.spec
	deployment, err := c.deploymentsLister.Deployments(book.Namespace).Get(deploymentName)
	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		deployment, err = c.kubeclientset.AppsV1().Deployments(book.Namespace).Create(ctx, newDeployment(book), metav1.CreateOptions{FieldManager: FieldManager})
	}

	// If an error occurs during Get/Create, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	// If the Deployment is not controlled by this book resource, we should log
	// a warning to the event recorder and return error msg.
	if !metav1.IsControlledBy(deployment, book) {
		msg := fmt.Sprintf(MessageResourceExists, deployment.Name)
		c.recorder.Event(book, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	// If this number of the replicas on the book resource is specified, and the
	// number does not equal the current desired replicas on the Deployment, we
	// should update the Deployment resource.
	// TODO: need to add more logic to make deployment update decision
	if (book.Spec.Replicas != nil && *book.Spec.Replicas != *deployment.Spec.Replicas) ||
		(book.Spec.Container.Image != "" && book.Spec.Container.Image != deployment.Spec.Template.Spec.Containers[0].Image ||
			(book.Spec.Container.Ports[0].ContainerPort != deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort)) {
		logger.V(4).Info("Update deployment resource", "currentReplicas", *deployment.Spec.Replicas, "desiredReplicas", *book.Spec.Replicas)
		deployment, err = c.kubeclientset.AppsV1().Deployments(book.Namespace).Update(ctx, newDeployment(book), metav1.UpdateOptions{FieldManager: FieldManager})
	}

	// If an error occurs during Update, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	svcName := book.Spec.DeploymentName + "service"
	_, err = c.serviceLister.Services(objectRef.Namespace).Get(svcName)
	if errors.IsNotFound(err) {
		_, err := c.kubeclientset.CoreV1().Services(objectRef.Namespace).Create(ctx, newService(book), metav1.CreateOptions{})
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	// TODO: need to add some checks before updating the service
	_, err = c.kubeclientset.CoreV1().Services(objectRef.Namespace).Update(ctx, newService(book), metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	envoyConfigMapName := book.Spec.DeploymentName + "-envoy-config"
	envoyConfigMap, err := c.kubeclientset.CoreV1().ConfigMaps(book.Namespace).Get(ctx, envoyConfigMapName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		envoyConfigMap, err = c.kubeclientset.CoreV1().ConfigMaps(book.Namespace).Create(ctx, newEnvoyConfigMap(book), metav1.CreateOptions{FieldManager: FieldManager})
	}

	if err != nil {
		return err
	}

	if !metav1.IsControlledBy(envoyConfigMap, book) {
		msg := fmt.Sprintf(MessageResourceExists, deployment.Name)
		c.recorder.Event(book, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	envoyDeploymentName := book.Spec.DeploymentName + "-envoy"

	envoyDeployment, err := c.deploymentsLister.Deployments(book.Namespace).Get(envoyDeploymentName)
	if errors.IsNotFound(err) {
		envoyDeployment, err = c.kubeclientset.AppsV1().Deployments(book.Namespace).Create(ctx, newEnvoyDeployment(book), metav1.CreateOptions{FieldManager: FieldManager})
	}
	// TODO: need to decide when we may need to update Envoy Deployment

	if err != nil {
		return err
	}

	if !metav1.IsControlledBy(envoyDeployment, book) {
		msg := fmt.Sprintf(MessageResourceExists, deployment.Name)
		c.recorder.Event(book, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	envoySvcName := book.Spec.DeploymentName + "-envoy-service"
	_, err = c.serviceLister.Services(objectRef.Namespace).Get(envoySvcName)
	if errors.IsNotFound(err) {
		_, err := c.kubeclientset.CoreV1().Services(objectRef.Namespace).Create(ctx, newEnvoyService(book), metav1.CreateOptions{})
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	// TODO: need to add some checks before updating the service
	_, err = c.kubeclientset.CoreV1().Services(objectRef.Namespace).Update(ctx, newEnvoyService(book), metav1.UpdateOptions{})

	if err != nil {
		return err
	}

	// Finally, we update the status block of the book resource to reflect the
	// current state of the world
	err = c.updateBookStatus(ctx, book, deployment)
	if err != nil {
		return err
	}

	c.recorder.Event(book, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

// enqueueBook takes a Book resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Book.
func (c *Controller) enqueueBook(obj interface{}) {
	if objectRef, err := cache.ObjectToName(obj); err != nil {
		utilruntime.HandleError(err)
		return
	} else {
		c.workqueue.Add(objectRef)
	}
}

func (c *Controller) updateBookStatus(ctx context.Context, book *bookv1.Book, deployment *appsv1.Deployment) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	bookCopy := book.DeepCopy()
	bookCopy.Status.AvailableReplicas = deployment.Status.AvailableReplicas
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the book resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := c.sampleclientset.SimplecustomcontrollerV1().Books(book.Namespace).UpdateStatus(ctx, bookCopy, metav1.UpdateOptions{FieldManager: FieldManager})
	return err
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the book resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that book resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	logger := klog.FromContext(context.Background())
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			// If the object value is not too big and does not contain sensitive information then
			// it may be useful to include it.
			utilruntime.HandleErrorWithContext(context.Background(), nil, "Error decoding object, invalid type", "type", fmt.Sprintf("%T", obj))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			// If the object value is not too big and does not contain sensitive information then
			// it may be useful to include it.
			utilruntime.HandleErrorWithContext(context.Background(), nil, "Error decoding object tombstone, invalid type", "type", fmt.Sprintf("%T", tombstone.Obj))
			return
		}
		logger.V(4).Info("Recovered deleted object", "resourceName", object.GetName())
	}
	logger.V(4).Info("Processing object", "object", klog.KObj(object))
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a book, we should not do anything more
		// with it.
		if ownerRef.Kind != "Book" {
			return
		}
		book, err := c.bookLister.Books(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			logger.V(4).Info("Ignore orphaned object", "object", klog.KObj(object), "book", ownerRef.Name)
			return
		}
		c.enqueueBook(book)
		return
	}
}

// newDeployment creates a new Deployment for a book resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the book resource that 'owns' it.
func newDeployment(book *bookv1.Book) *appsv1.Deployment {
	labels := map[string]string{
		"app":        "book-server",
		"controller": book.Name,
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      book.Spec.DeploymentName,
			Namespace: book.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(book, bookv1.SchemeGroupVersion.WithKind("Book")),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: book.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						book.Spec.Container,
					},
				},
			},
		},
	}
}

func newEnvoyDeployment(book *bookv1.Book) *appsv1.Deployment {
	labels := map[string]string{
		"app":        "envoy",
		"controller": book.Name,
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      book.Spec.DeploymentName + "-envoy",
			Namespace: book.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(book, bookv1.SchemeGroupVersion.WithKind("Book")),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: book.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  book.Spec.DeploymentName + "-envoy",
							Image: "envoyproxy/envoy:v1.32.3",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 1999,
								},
								{
									Name:          "admin",
									ContainerPort: 8001,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "envoy-config",
									MountPath: "/etc/envoy",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "envoy-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: book.Spec.DeploymentName + "-envoy-config",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func newService(book *bookv1.Book) *corev1.Service {
	labels := map[string]string{
		"app":        "book-server",
		"controller": book.Name,
	}
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: book.Spec.DeploymentName + "service",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(book, bookv1.SchemeGroupVersion.WithKind("Book")),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port:       book.Spec.Container.Ports[0].ContainerPort,
					TargetPort: intstr.FromInt32(book.Spec.Container.Ports[0].ContainerPort),
					NodePort:   30009,
				},
			},
		},
	}
}

func newEnvoyService(book *bookv1.Book) *corev1.Service {
	labels := map[string]string{
		"app":        "envoy",
		"controller": book.Name,
	}
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: book.Spec.DeploymentName + "-envoy-service",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(book, bookv1.SchemeGroupVersion.WithKind("Book")),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port:       1999,
					TargetPort: intstr.FromInt32(1999),
				},
			},
		},
	}
}

func newEnvoyConfigMap(book *bookv1.Book) *corev1.ConfigMap {
	//labels := map[string]string{
	//	"app":        "book-server",
	//	"controller": book.Name,
	//}
	wd, _ := os.Getwd()
	envoyConfig, err := os.ReadFile(wd + "/manifests/envoy.yaml")
	if err != nil {
		panic(err.Error())
	}
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind: "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: book.Spec.DeploymentName + "-envoy-config",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(book, bookv1.SchemeGroupVersion.WithKind("Book")),
			},
		},
		Data: map[string]string{
			"envoy.yaml": string(envoyConfig),
		},
	}
}
