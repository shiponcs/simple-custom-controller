package utils

import (
	"context"
	"fmt"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"sigs.k8s.io/yaml"
)

func CreateCRD(Clientset *apiextensionsclientset.Clientset) error {
	wd, _ := os.Getwd()

	crdYaml, err := os.ReadFile(wd + "/manifests/simplecustomcontroller.crd.com_books.yaml")

	if err != nil {
		panic(err)
	}

	var crd apiextensionsv1.CustomResourceDefinition
	err = yaml.Unmarshal(crdYaml, &crd)
	if err != nil {
		panic(err)
	}

	existingCRD, err := Clientset.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), crd.Name, metav1.GetOptions{})
	if err == nil {
		// the CRD exists
		fmt.Printf("CRD %s already exists, Updating\n", crd.Name)
		crd.ResourceVersion = existingCRD.ResourceVersion
		_, err := Clientset.ApiextensionsV1().CustomResourceDefinitions().Update(context.TODO(), &crd, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		return nil
	} else if !errors.IsNotFound(err) {
		// there is something wrong
		panic(fmt.Errorf("error checking CRD %s: %v", crd.Name, err))
		return err
	}
	// the CRD doesn't exist
	fmt.Printf("Creating CRD %s\n", crd.Name)
	_, err = Clientset.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), &crd, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}