package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestRootCmd_ExecutesWithoutError(t *testing.T) {
	assert := is.New(t)
	cmd := newRootCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.NoErr(err)
}

func TestRootCmd_HelpContainsDescription(t *testing.T) {
	assert := is.New(t)
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	assert.NoErr(err)
	assert.True(strings.Contains(buf.String(), "fast enough Python package and project manager"))
}

func TestVersionCmd_PrintsVersion(t *testing.T) {
	assert := is.New(t)
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})
	err := cmd.Execute()
	assert.NoErr(err)
	assert.True(strings.Contains(buf.String(), "pensa dev"))
}

func TestVersionCmd_PrintsCustomVersion(t *testing.T) {
	assert := is.New(t)
	original := Version
	Version = "1.2.3"
	defer func() { Version = original }()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})
	err := cmd.Execute()
	assert.NoErr(err)
	assert.True(strings.Contains(buf.String(), "pensa 1.2.3"))
}
