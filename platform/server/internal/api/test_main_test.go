package api

import (
	"os"
	"testing"
)

func TestMain(tests *testing.M) {
	if err := os.Setenv("PLATFORM_INSECURE_DEV_AUTH", "1"); err != nil {
		panic(err)
	}
	os.Exit(tests.Run())
}
