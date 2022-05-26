package logger

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
)

func Test_SharedRotater(t *testing.T) {
	stdout, err := SharedRotater("stdout", config.Rotation{})
	if err != nil {
		log.Fatal(err)
	}
	defer stdout.Close()

	stdout2, err := SharedRotater("stdout", config.Rotation{})
	if err != nil {
		log.Fatal(err)
	}
	defer stdout2.Close()

	if stdout != stdout2 {
		t.Errorf("unexpected loggers: %#v != %#v", stdout, stdout2)
	}

	stderr, err := SharedRotater("stderr", config.Rotation{})
	if err != nil {
		log.Fatal(err)
	}
	defer stderr.Close()

	if len(sharedRotaters) != 2 {
		t.Errorf("unexpected sharedRotaters length: %d; want 2", len(sharedRotaters))
	}

	stdout.Close()
	stdout2.Close()
	stderr.Close()
	if len(sharedRotaters) != 0 {
		t.Errorf("unexpected sharedRotaters length: %d; want 0", len(sharedRotaters))
	}
}

func Test_RotateShared(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "*.logger_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	output := filepath.Join(tmpDir, "test.log")
	o, err := SharedRotater(output, config.Rotation{
		MaxSize:    1,
		MaxBackups: 2,
		MaxAge:     3,
		Compress:   true,
		LocalTime:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()

	o2, err := SharedRotater("stdout", config.Rotation{})
	if err != nil {
		t.Fatal(err)
	}
	defer o2.Close()

	_, err = o.Write([]byte("test\n"))
	if err != nil {
		t.Fatal(err)
	}

	if err := RotateShared(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("unexpected entries size: %d; want 2", len(entries))
	}
}
