package main

import (
	//"context"
	//"fmt"
	//clientset "github.com/shiponcs/simple-custom-controller/pkg/generated/clientset/versioned"
	"github.com/shiponcs/simple-custom-controller/utils"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	cfg, err := clientcmd.BuildConfigFromFlags("", homedir.HomeDir()+"/.kube/config")
	if err != nil {
		panic(err.Error())
	}
	//bookClient, err := clientset.NewForConfig(cfg)
	//kubeClient, err := kubernetes.NewForConfig(cfg)
	extensionsClient, err := apiextensionsclientset.NewForConfig(cfg)
	if err := utils.CreateCRD(extensionsClient); err != nil {
		panic(err.Error())
	}
}
