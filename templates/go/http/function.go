package function

import (
	"context"
	"fmt"
	"net/http"
)

// MyFunction is the function provided by this library.
// There is no restriction as to the actual name of this structure.
type MyFunction struct{}

// New constructs an instance of your function.  It is called each time a new
// instance of the function service is created.  The structure returned need
// only provide a Handle method (see example below).
func New() *MyFunction {
	return &MyFunction{}
}

// Start your function instance.
// Provided to this start method are all arguments and environment variables
// which apply to this function. While this method technically is optional,
// it is preferable to receive function configuration in this way rather than
// access them directory for testability, portabiliy, simplicity and robustness.
func (f *MyFunction) Start(ctx context.Context, args map[string]string) error {
	fmt.Println("Function Started")
	return nil
}

// Handle a request using your funciton instance.
func (f *MyFunction) Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	fmt.Println("Function Received Request")
	fmt.Fprintf(res, "Function Received Request")
}

// Stop is an optional method which is called whenever a function is stopped.
// This may happen for reasons such as being rescheduled onto a different node,
// being updated with a newer version, or if the number of function instances
// is being scaled down due to low load.  This is a good place to cleanup and
// realease any resources which expect to be manually released.
//
// func (f *Function) Stop(ctx context.Context) error { return nil }

// Alive is an optional method which allows you to more deeply indicate that
// your funciton is alive.  The default liveness implementation returns true
// if the funciton process is not deadlocked and able to respond.  A custom
// implementation of this method may be useful when a function should not be
// considered alive if any dependent services are alive, or other more
// complex logic.
//
// func (f *Function) Alive(ctx context.Context) (bool, error) {
//   return true, nil
// }

// Ready is an optional method which, when implemented, will ensure that
// requests are not made to the Function's request handler until this method
// reports true.
//
// func (f *Function) Ready(ctx context.Context) (bool, error) {
//   return true, nil
// }

// Handle is an optional method which can be used to implement simple functions
// with little or no state, and minimal testing requirements.  By implementing
// this package static function, one can forego the constructor and struct
// outlined above.  Note that if this method is defined, the system will ignore
// the instanced function constructor if it is defined.
//
// func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request {
//   /* Your Static Handler Code Here */
// }
