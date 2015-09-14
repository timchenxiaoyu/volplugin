package volplugin

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/contiv/volplugin/cephdriver"
	"github.com/contiv/volplugin/config"
	"github.com/docker/docker/pkg/plugins"
)

func nilAction(w http.ResponseWriter, r *http.Request) {
	content, err := json.Marshal(VolumeResponse{})
	if err != nil {
		httpError(w, "Could not marshal request", err)
		return
	}
	w.Write(content)
}

func activate(w http.ResponseWriter, r *http.Request) {
	content, err := json.Marshal(plugins.Manifest{Implements: []string{"VolumeDriver"}})
	if err != nil {
		httpError(w, "Could not generate bootstrap response", err)
		return
	}

	w.Write(content)
}

func deactivate(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
}

func create(master, tenantName string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vr, err := unmarshalRequest(r.Body)
		if err != nil {
			httpError(w, "Could not unmarshal request", err)
			return
		}

		if vr.Name == "" {
			httpError(w, "Image name is empty", nil)
			return
		}

		if err := requestCreate(master, tenantName, vr.Name); err != nil {
			httpError(w, "Could not determine tenant configuration", err)
			return
		}

		content, err := marshalResponse(VolumeResponse{Mountpoint: vr.Name, Err: ""})
		if err != nil {
			httpError(w, "Could not marshal response", err)
			return
		}

		w.Write(content)
	}
}

func getPath(master string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vr, err := unmarshalRequest(r.Body)
		if err != nil {
			httpError(w, "Could not unmarshal request", err)
			return
		}

		if vr.Name == "" {
			httpError(w, "Name is empty", nil)
			return
		}

		log.Infof("Returning mount path to docker for volume: %q", vr.Name)

		config, err := requestTenantConfig(master, vr.Name)
		if err != nil {
			httpError(w, "Could not determine tenant configuration", err)
			return
		}

		// FIXME need to ensure that the mount exists before returning to docker
		driver := cephdriver.NewCephDriver(config.Pool)

		content, err := marshalResponse(VolumeResponse{Mountpoint: driver.MountPath(vr.Name)})
		if err != nil {
			httpError(w, "Reply could not be marshalled", err)
			return
		}

		w.Write(content)
	}
}

func mount(master string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vr, err := unmarshalRequest(r.Body)
		if err != nil {
			httpError(w, "Could not unmarshal request", err)
			return
		}

		if vr.Name == "" {
			httpError(w, "Name is empty", nil)
			return
		}

		// FIXME check if we're holding the mount already
		log.Infof("Mounting volume %q", vr.Name)

		tenConfig, err := requestTenantConfig(master, vr.Name)
		if err != nil {
			httpError(w, "Could not determine tenant configuration", err)
			return
		}

		driver := cephdriver.NewCephDriver(tenConfig.Pool)

		if err := driver.NewVolume(vr.Name, tenConfig.Size).Mount(); err != nil {
			httpError(w, "Volume could not be mounted", err)
			return
		}

		hostname, err := os.Hostname()
		if err != nil {
			httpError(w, "Retrieving hostname", err)
			return
		}

		mt := &config.MountConfig{
			Volume:     vr.Name,
			MountPoint: driver.MountPath(vr.Name),
			Host:       hostname,
		}

		if err := reportMount(master, mt); err != nil {
			httpError(w, "Reporting mount to master", err)
			return
		}

		content, err := marshalResponse(VolumeResponse{Mountpoint: driver.MountPath(vr.Name)})
		if err != nil {
			httpError(w, "Reply could not be marshalled", err)
			return
		}

		w.Write(content)
	}
}

func unmount(master string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vr, err := unmarshalRequest(r.Body)
		if err != nil {
			httpError(w, "Could not unmarshal request", err)
			return
		}

		if vr.Name == "" {
			httpError(w, "Name is empty", nil)
			return
		}

		log.Infof("Unmounting volume %q", vr.Name)

		tenConfig, err := requestTenantConfig(master, vr.Name)
		if err != nil {
			httpError(w, "Could not determine tenant configuration", err)
			return
		}

		driver := cephdriver.NewCephDriver(tenConfig.Pool)

		if err := driver.NewVolume(vr.Name, tenConfig.Size).Unmount(); err != nil {
			httpError(w, "Could not mount image", err)
			return
		}

		hostname, err := os.Hostname()
		if err != nil {
			httpError(w, "Retrieving hostname", err)
			return
		}

		mt := &config.MountConfig{
			Volume:     vr.Name,
			MountPoint: driver.MountPath(vr.Name),
			Host:       hostname,
		}

		if err := reportUnmount(master, mt); err != nil {
			httpError(w, "Reporting unmount to master", err)
			return
		}

		content, err := marshalResponse(VolumeResponse{Mountpoint: driver.MountPath(vr.Name)})
		if err != nil {
			httpError(w, "Reply could not be marshalled", err)
			return
		}

		w.Write(content)
	}
}

// Catchall for additional driver functions.
func action(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Unknown driver action at %q", r.URL.Path)
	content, _ := ioutil.ReadAll(r.Body)
	log.Debug("Body content:", string(content))
	w.WriteHeader(503)
}
