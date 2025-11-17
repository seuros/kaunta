package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunPassesEmbeddedAssetsToCLI(t *testing.T) {
	original := executeCLI
	defer func() { executeCLI = original }()

	called := false
	executeCLI = func(
		version string,
		gotAssetsFS interface{},
		gotTracker []byte,
		gotVendorJS []byte,
		gotVendorCSS []byte,
		gotGeoJSON []byte,
		gotViewsFS interface{},
		gotSetup []byte,
		gotSetupComplete []byte,
	) error {
		called = true
		assert.Equal(t, strings.TrimSpace(versionFile), version)
		assert.NotNil(t, gotAssetsFS)
		assert.Equal(t, trackerScript, gotTracker)
		assert.Equal(t, vendorJS, gotVendorJS)
		assert.Equal(t, vendorCSS, gotVendorCSS)
		assert.Equal(t, countriesGeoJSON, gotGeoJSON)
		assert.NotNil(t, gotViewsFS)
		assert.Equal(t, setupTemplate, gotSetup)
		assert.Equal(t, setupCompleteTemplate, gotSetupComplete)
		return nil
	}

	require.NoError(t, run())
	assert.True(t, called)
}

func TestRunPropagatesExecuteError(t *testing.T) {
	original := executeCLI
	defer func() { executeCLI = original }()

	executeCLI = func(
		version string,
		assetsFS interface{},
		tracker []byte,
		vendorJSBytes []byte,
		vendorCSSBytes []byte,
		geoJSON []byte,
		viewsFS interface{},
		setup []byte,
		setupComplete []byte,
	) error {
		return errors.New("boom")
	}

	err := run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}
