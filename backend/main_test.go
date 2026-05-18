package main

import (
	"os"
	"strings"
	"testing"
)

func TestMainStartsScheduler(t *testing.T) {
	content, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(content)
	if !strings.Contains(src, "scheduler.NewScheduler(taskSvc, repoSvc)") || !strings.Contains(src, ".Start()") {
		t.Fatal("main should create and start the task scheduler")
	}
}
