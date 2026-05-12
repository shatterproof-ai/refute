package main

import "example.com/renametest/util"

func main() {
	msg := util.BuildGreeting("world")
	result := 6*7 + 1
	println(msg, result)
	var u *util.User = util.NewUser("world")
	println(u.Name)
}
