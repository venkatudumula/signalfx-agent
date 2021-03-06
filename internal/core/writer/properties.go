package writer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/signalfx/signalfx-agent/internal/core/config"
	"github.com/signalfx/signalfx-agent/internal/monitors/types"
	"github.com/signalfx/signalfx-agent/internal/utils"
	log "github.com/sirupsen/logrus"
)

type dimensionPropertyClient struct {
	client *http.Client
	Token  string
	APIURL *url.URL
	// Keeps track of what has been synced so we don't do unnecessary syncs
	history *lru.Cache
	lock    sync.Mutex
	// A buffered channel that mimics a semaphore when performance isn't that
	// big of a deal.
	reqSema chan struct{}

	TotalPropUpdates int64
}

func newDimensionPropertyClient(conf *config.WriterConfig) *dimensionPropertyClient {
	history, err := lru.New(int(conf.PropertiesHistorySize))
	if err != nil {
		panic("could not create properties history cache: " + err.Error())
	}

	return &dimensionPropertyClient{
		Token:  conf.SignalFxAccessToken,
		APIURL: conf.APIURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 90 * time.Second,
					DualStack: true,
				}).DialContext,
				MaxIdleConns:        int(conf.PropertiesMaxRequests),
				MaxIdleConnsPerHost: int(conf.PropertiesMaxRequests),
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
		history: history,
		reqSema: make(chan struct{}, int(conf.PropertiesMaxRequests)),
	}
}

// SetPropertiesOnDimension will set custom properties on a specific dimension
// value.  It will wipe out any description or tags on the dimension.  There is
// no retry logic here so any failures are terminal.
func (dpc *dimensionPropertyClient) SetPropertiesOnDimension(dimProps *types.DimProperties) error {
	if !dpc.isDuplicate(dimProps) {
		dpc.reqSema <- struct{}{}

		log.WithFields(log.Fields{
			"name":  dimProps.Name,
			"value": dimProps.Value,
			"props": dimProps.Properties,
			"tags":  dimProps.Tags,
		}).Info("Syncing properties to dimension")

		err := dpc.doReq(dimProps.Name, dimProps.Value, dimProps.Properties, dimProps.Tags)
		<-dpc.reqSema
		if err != nil {
			return err
		}
		// Add it to the history only after successfully propagated.  This
		// could lead to some duplicates if there are multiple concurrent calls
		// for the same dim props, but that's ok.
		dpc.history.Add(dimProps.Dimension, dimProps)
		atomic.AddInt64(&dpc.TotalPropUpdates, int64(1))
	}
	return nil
}

// isDuplicate returns true if the exact same dimension properties have been
// synced before in the recent past.
func (dpc *dimensionPropertyClient) isDuplicate(dimProps *types.DimProperties) bool {
	prev, ok := dpc.history.Get(dimProps.Dimension)
	return ok && reflect.DeepEqual(prev.(*types.DimProperties), dimProps)
}

func (dpc *dimensionPropertyClient) doReq(key, value string, props map[string]string, tags map[string]bool) error {
	json, err := json.Marshal(map[string]interface{}{
		"key":              key,
		"value":            value,
		"customProperties": props,
		"tags":             utils.StringSetToSlice(tags),
	})
	if err != nil {
		return err
	}

	url, err := dpc.APIURL.Parse(fmt.Sprintf("/v2/dimension/%s/%s", key, value))
	if err != nil {
		return errors.Wrapf(err, "Could not construct dimension property PUT URL with %s / %s", key, value)
	}

	req, err := http.NewRequest(
		"PUT",
		url.String(),
		bytes.NewReader(json))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-SF-TOKEN", dpc.Token)

	resp, err := dpc.client.Do(req)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("Unexpected status code %d on response %s", resp.StatusCode, string(body))
	}

	return err
}
