package main

import "example.com/renametest/util"

func main() {
	msg := util.FormatGreeting("world")
	println(msg)
	var u *util.User = util.NewUser("world")
	println(u.Name)
}
