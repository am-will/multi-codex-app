//go:build !(darwin || linux)

package main

func codexHomeInUse(string) (bool, error) { return false, sharingSupported() }
