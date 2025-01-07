package main

import (
	"context"
	"fmt"
	clientset "github.com/shiponcs/simple-custom-controller/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	cfg, err := clientcmd.BuildConfigFromFlags("", homedir.HomeDir()+"/.kube/config")
	if err != nil {
		panic(err.Error())
	}
	bookClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		panic(err.Error())
	}

	book, err := bookClient.SimplecustomcontrollerV1().Books("default").Get(context.TODO(), "example-book", metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}
	rep := book.Spec.Replicas
	fmt.Printf("The replica number is %d\n", *rep)
}
