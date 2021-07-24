package config

import (
	"os"
	"testing"
	"time"

	"github.com/hashicorp/logutils"
	"github.com/stretchr/testify/assert"
)

func testConfig(t *testing.T) *Config {
	t.Helper()
	return &Config{
		Base: Base{
			LogLevel:                  "INFO",
			SetSize:                   2,
			Threshold:                 10,
			StartRank:                 1,
			ValidatorListenAddress:    "tcp://127.0.0.1:3000",
			ValidatorListenAddressRPC: "tcp://127.0.0.1:26657",
			RetryDialAfter:            "15s",
			BootStrapTime:             "10m",
		},
		Privval: PrivValidator{
			ChainID: "testchain",
		},
	}
}

func testInvalidBase(t *testing.T, base Base) {
	// Invalid Base.LogLevel.
	base.LogLevel = "INVALID"
	err := base.validate()
	assert.Error(t, err)
	base.LogLevel = testConfig(t).Base.LogLevel

	// Invalid Base.SetSize.
	base.SetSize = 0
	err = base.validate()
	assert.Error(t, err)
	base.SetSize = testConfig(t).Base.SetSize

	// Invalid Base.Threshold.
	base.Threshold = 1
	err = base.validate()
	assert.Error(t, err)
	base.Threshold = testConfig(t).Base.Threshold

	// Invalid Base.StartRank.
	base.StartRank = 0
	err = base.validate()
	assert.Error(t, err)
	base.StartRank = testConfig(t).Base.StartRank

	// Invalid protocol in Base.ValidatorListenAddress.
	base.ValidatorListenAddress = "invalid://127.0.0.1:3000"
	err = base.validate()
	assert.Error(t, err)
	base.ValidatorListenAddress = testConfig(t).Base.ValidatorListenAddress

	// Valid protocol (unix), but invalid suffix in Base.ValidatorListenAddress.
	base.ValidatorListenAddress = "unix:///test"
	err = base.validate()
	assert.Error(t, err)
	base.ValidatorListenAddress = testConfig(t).Base.ValidatorListenAddress

	// Invalid host:port format in Base.ValidatorListenAddress.
	base.ValidatorListenAddress = "tcp://127.0.0.1"
	err = base.validate()
	assert.Error(t, err)
	base.ValidatorListenAddress = testConfig(t).Base.ValidatorListenAddress

	// Invalid IPv4 address in Base.ValidatorListenAddress.
	base.ValidatorListenAddress = "tcp://127.300.0.1:3000"
	err = base.validate()
	assert.Error(t, err)
	base.ValidatorListenAddress = testConfig(t).Base.ValidatorListenAddress

	// Invalid protocol in Base.ValidatorListenAddressRPC.
	base.ValidatorListenAddressRPC = "invalid://127.0.0.1:26657"
	err = base.validate()
	assert.Error(t, err)
	base.ValidatorListenAddressRPC = testConfig(t).Base.ValidatorListenAddressRPC

	// Invalid host:port format in Base.ValidatorListenAddressRPC.
	base.ValidatorListenAddressRPC = "tcp://127.0.0.1"
	err = base.validate()
	assert.Error(t, err)
	base.ValidatorListenAddressRPC = testConfig(t).Base.ValidatorListenAddressRPC

	// Invalid IPv4 address in Base.ValidatorListenAddressRPC.
	base.ValidatorListenAddressRPC = "tcp://127.300.0.1:26657"
	err = base.validate()
	assert.Error(t, err)
	base.ValidatorListenAddressRPC = testConfig(t).Base.ValidatorListenAddressRPC

	// Invalid Base.RetryDialAfter (empty).
	base.RetryDialAfter = ""
	err = base.validate()
	assert.Error(t, err)
	base.RetryDialAfter = testConfig(t).Base.RetryDialAfter

	// Invalid format in Base.RetryDialAfter.
	base.RetryDialAfter = "01d"
	err = base.validate()
	assert.Error(t, err)
	base.RetryDialAfter = testConfig(t).Base.RetryDialAfter
}

func testInvalidPrivValidator(t *testing.T, privval PrivValidator) {
	// Invalid PrivValidator.ChainID.
	privval.ChainID = ""
	err := privval.validate()
	assert.Error(t, err)
	privval.ChainID = testConfig(t).Privval.ChainID
}

func TestValidateConfig(t *testing.T) {
	// Valid Config.
	cfg := testConfig(t)
	err := cfg.validate()
	assert.NoError(t, err)

	// Invalid Config.
	cfg.Base.LogLevel = "INVALID"
	cfg.Privval.ChainID = ""
	err = cfg.validate()
	assert.Error(t, err)

	// Invalid Config.
	testInvalidBase(t, cfg.Base)
	testInvalidPrivValidator(t, cfg.Privval)
}

func TestDir(t *testing.T) {
	os.Setenv("SIGNCTRL_CONFIG_DIR", "/tmp")
	dir := Dir()
	assert.Equal(t, "/tmp", dir)

	os.Unsetenv("SIGNCTRL_CONFIG_DIR")
	dir = Dir()
	homeDir, _ := os.UserHomeDir()
	assert.Equal(t, homeDir+"/.signctrl", dir)

	os.Unsetenv("HOME")
	dir = Dir()
	assert.Equal(t, ".", dir)
}

func TestFilePath(t *testing.T) {
	path := FilePath("/tmp")
	assert.Equal(t, "/tmp/config.toml", path)
}

func TestGetRetryDialTime(t *testing.T) {
	dur := GetRetryDialTime("3600s")
	assert.Equal(t, 3600*time.Second, dur)

	dur = GetRetryDialTime("60m")
	assert.Equal(t, 60*time.Minute, dur)

	dur = GetRetryDialTime("1h")
	assert.Equal(t, time.Hour, dur)

	dur = GetRetryDialTime("01h")
	assert.Equal(t, time.Duration(0), dur)

	dur = GetRetryDialTime("1d")
	assert.Equal(t, time.Duration(0), dur)
}

func TestLogLevelsToRegExp(t *testing.T) {
	lvls := []logutils.LogLevel{"A", "BC", "DEF"}
	regexp := logLevelsToRegExp(&lvls)
	assert.Equal(t, "A|BC|DEF", regexp)
}

func TestValidate_time(t *testing.T) {
	testCases := []struct {
		name       string
		time_value string
		expPass    bool
	}{
		{"incorrect time format abc", "abc", false},
		{"incorect time format suffix sa instead of s,m,h", "12sa", false},
		{"incorect time format suffix mf instead of s,m,h", "12mf", false},
		{"incorect time format suffix hg instead of s,m,h", "12hg", false},
		{"incorrect time number", "1a2h", false},
		{"correct time format 10s", "10s", true},
		{"correct time format 1234m", "1234m", true},
		{"correct time format 7890h", "7890h", true},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			errs := validate_time("", "config_time_attribute", tt.time_value)
			if tt.expPass {
				assert.True(t, errs == "")
			} else {
				assert.False(t, errs == "")
			}
		})
	}

}
