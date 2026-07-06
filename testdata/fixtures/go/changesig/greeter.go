package main

// render builds a greeting. The verbosity parameter is unused and can be
// removed with change-signature; name is used and cannot.
func render(name string, verbosity int) string {
	return "hello " + name
}
