package main

// useLocal calls render from a second file so change-signature must rewrite a
// cross-file call site, not only the declaration.
func useLocal() string {
	return render("local", 1)
}
