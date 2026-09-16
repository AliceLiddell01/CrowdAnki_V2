package main

import "testing"

func TestAppVersion(t *testing.T) {
	if appVersion == "" {
		t.Fatal("версия приложения не должна быть пустой строкой")
	}
}
