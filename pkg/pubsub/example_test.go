package pubsub

import "fmt"

func ExampleResult() {
	r1 := Result[int]{Ok: 1}
	r2 := Result[int]{Err: fmt.Errorf("error")}

	if r1.Err != nil {
		fmt.Println("r1 has error")
		return
	}
	fmt.Println(r1)

	if r2.Err != nil {
		fmt.Println("r2 has error")
	} else if v := r2.Ok; v == 0 {
		fmt.Println("r2 has zero value")
	} else {
		fmt.Println("r2 has value", v)
	}

	switch {
	case r2.Err != nil:
		fmt.Println("r2 has error")
	case r2.Ok == 0:
		fmt.Println("r2 has zero value")
	default:
		fmt.Println("r2 has value", r2.Ok)
	}

	// Output:
	// {1 <nil>}
	// r2 has error
	// r2 has error
}
