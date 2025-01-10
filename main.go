package main

import (
	"flag"
	"github.com/shiponcs/simple-custom-controller/controller"
	clientset "github.com/shiponcs/simple-custom-controller/pkg/generated/clientset/versioned"
	_ "github.com/shiponcs/simple-custom-controller/pkg/generated/informers/externalversions/simplecustomcontroller/v1"
	"github.com/shiponcs/simple-custom-controller/pkg/signals"
	_ "golang.org/x/time/rate"
	_ "k8s.io/api/apps/v1"
	_ "k8s.io/apimachinery/pkg/util/runtime"
	_ "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	_ "k8s.io/client-go/tools/cache"
	_ "k8s.io/client-go/tools/record"
	_ "k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	_ "k8s.io/klog/v2"
	_ "k8s.io/sample-controller/pkg/generated/clientset/versioned/scheme"
	"time"

	//"context"
	//"fmt"
	bookInformers "github.com/shiponcs/simple-custom-controller/pkg/generated/informers/externalversions"
	"github.com/shiponcs/simple-custom-controller/utils"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	klog.InitFlags(nil)
	ctx := signals.SetupSignalHandler()
	logger := klog.FromContext(ctx)

	var kubeconfig string
	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.Parse()

	var cfg *rest.Config
	var err error

	if kubeconfig == "" {
		logger.V(4).Info("Running in in-cluster mode")
		cfg, err = rest.InClusterConfig()
		if err != nil {
			logger.V(4).Error(err, "Error running in in-cluster mode")
			return
		}
	} else {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			logger.V(4).Error(err, "Error running in out-cluster mode")
		}
	}

	// create the CRD from the yaml file
	extensionsClient, err := apiextensionsclientset.NewForConfig(cfg)
	if err := utils.CreateCRD(extensionsClient); err != nil {
		panic(err.Error())
	}

	// wait for the crd creation to complete
	if err := utils.WaitForCRDEstablishment(extensionsClient, "books.simplecustomcontroller.crd.com"); err != nil {
		panic(err.Error())
		return
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err.Error())
	}

	bookClient, err := clientset.NewForConfig(cfg) // client for our book CRD
	if err != nil {
		panic(err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	bookInformerFactory := bookInformers.NewSharedInformerFactory(bookClient, time.Second*30)

	controller := controller.NewController(ctx, kubeClient, bookClient,
		kubeInformerFactory.Apps().V1().Deployments(),
		kubeInformerFactory.Core().V1().Services(),
		bookInformerFactory.Simplecustomcontroller().V1().Books())

	kubeInformerFactory.Start(ctx.Done())
	bookInformerFactory.Start(ctx.Done())

	if err := controller.Run(ctx, 2); err != nil {
		logger.Error(err, "Error running controller")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
}
