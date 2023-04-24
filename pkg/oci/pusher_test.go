package oci

import "testing"

func TestPusher(t *testing.T) {
	// Create a function
	// Build the function
	// Create an HTTP listener on an OS-chosen port ${port}
	// Create an endpoint which emulates an OCI compliant push-target
	// Alter the funciton's image tag to be localhost:${port}
	// Create an OCI pusher instance and .Push
	// Confirm with the HTTP listener that the expected request was received
	//  - Only a shallow test is necessary because the unit tests of
	//    go-containerregistry/pkg/v1/remote ensure it is correct
	// Note that the test may need to use the explicit username and password
}
