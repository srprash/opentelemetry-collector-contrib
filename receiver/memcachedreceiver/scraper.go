// Copyright 2020, OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memcachedreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/memcachedreceiver"

import (
	"context"
	"strconv"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/memcachedreceiver/internal/metadata"
)

const (
	emitMetricsWithDirectionAttributeFeatureGateID    = "receiver.memcached.emitMetricsWithDirectionAttribute"
	emitMetricsWithoutDirectionAttributeFeatureGateID = "receiver.memcached.emitMetricsWithoutDirectionAttribute"
)

var (
	emitMetricsWithDirectionAttributeFeatureGate = featuregate.Gate{
		ID:      emitMetricsWithDirectionAttributeFeatureGateID,
		Enabled: true,
		Description: "Some memcached metrics reported are transitioning from being reported with a direction " +
			"attribute to being reported with the direction included in the metric name to adhere to the " +
			"OpenTelemetry specification. This feature gate controls emitting the old metrics with the direction " +
			"attribute. For more details, see: " +
			"https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/memcachedreceiver/README.md#feature-gate-configurations",
	}

	emitMetricsWithoutDirectionAttributeFeatureGate = featuregate.Gate{
		ID:      emitMetricsWithoutDirectionAttributeFeatureGateID,
		Enabled: false,
		Description: "Some memcached metrics reported are transitioning from being reported with a direction " +
			"attribute to being reported with the direction included in the metric name to adhere to the " +
			"OpenTelemetry specification. This feature gate controls emitting the new metrics without the direction " +
			"attribute. For more details, see: " +
			"https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/memcachedreceiver/README.md#feature-gate-configurations",
	}
)

func init() {
	featuregate.GetRegistry().MustRegister(emitMetricsWithDirectionAttributeFeatureGate)
	featuregate.GetRegistry().MustRegister(emitMetricsWithoutDirectionAttributeFeatureGate)
}

type memcachedScraper struct {
	logger                               *zap.Logger
	config                               *Config
	mb                                   *metadata.MetricsBuilder
	newClient                            newMemcachedClientFunc
	emitMetricsWithDirectionAttribute    bool
	emitMetricsWithoutDirectionAttribute bool
}

func newMemcachedScraper(
	settings component.ReceiverCreateSettings,
	config *Config,
) memcachedScraper {
	return memcachedScraper{
		logger:                               settings.Logger,
		config:                               config,
		newClient:                            newMemcachedClient,
		mb:                                   metadata.NewMetricsBuilder(config.Metrics, settings.BuildInfo),
		emitMetricsWithDirectionAttribute:    featuregate.GetRegistry().IsEnabled(emitMetricsWithDirectionAttributeFeatureGateID),
		emitMetricsWithoutDirectionAttribute: featuregate.GetRegistry().IsEnabled(emitMetricsWithoutDirectionAttributeFeatureGateID),
	}
}

func (r *memcachedScraper) scrape(_ context.Context) (pmetric.Metrics, error) {
	// Init client in scrape method in case there are transient errors in the
	// constructor.
	statsClient, err := r.newClient(r.config.Endpoint, r.config.Timeout)
	if err != nil {
		r.logger.Error("Failed to establish client", zap.Error(err))
		return pmetric.Metrics{}, err
	}

	allServerStats, err := statsClient.Stats()
	if err != nil {
		r.logger.Error("Failed to fetch memcached stats", zap.Error(err))
		return pmetric.Metrics{}, err
	}

	now := pcommon.NewTimestampFromTime(time.Now())

	for _, stats := range allServerStats {
		for k, v := range stats.Stats {
			switch k {
			case "bytes":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedBytesDataPoint(now, parsedV)
				}
			case "curr_connections":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedConnectionsCurrentDataPoint(now, parsedV)
				}
			case "total_connections":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedConnectionsTotalDataPoint(now, parsedV)
				}
			case "cmd_get":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedCommandsDataPoint(now, parsedV, metadata.AttributeCommandGet)
				}
			case "cmd_set":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedCommandsDataPoint(now, parsedV, metadata.AttributeCommandSet)
				}
			case "cmd_flush":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedCommandsDataPoint(now, parsedV, metadata.AttributeCommandFlush)
				}
			case "cmd_touch":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedCommandsDataPoint(now, parsedV, metadata.AttributeCommandTouch)
				}
			case "curr_items":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedCurrentItemsDataPoint(now, parsedV)
				}

			case "threads":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedThreadsDataPoint(now, parsedV)
				}

			case "evictions":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedEvictionsDataPoint(now, parsedV)
				}
			case "bytes_read":
				if parsedV, ok := r.parseInt(k, v); ok {
					if r.emitMetricsWithDirectionAttribute {
						r.mb.RecordMemcachedNetworkDataPoint(now, parsedV, metadata.AttributeDirectionReceived)
					}
					if r.emitMetricsWithoutDirectionAttribute {
						r.mb.RecordMemcachedNetworkReceivedDataPoint(now, parsedV)
					}
				}
			case "bytes_written":
				if parsedV, ok := r.parseInt(k, v); ok {
					if r.emitMetricsWithDirectionAttribute {
						r.mb.RecordMemcachedNetworkDataPoint(now, parsedV, metadata.AttributeDirectionSent)
					}
					if r.emitMetricsWithoutDirectionAttribute {
						r.mb.RecordMemcachedNetworkSentDataPoint(now, parsedV)
					}
				}
			case "get_hits":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedOperationsDataPoint(now, parsedV, metadata.AttributeTypeHit,
						metadata.AttributeOperationGet)
				}
			case "get_misses":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedOperationsDataPoint(now, parsedV, metadata.AttributeTypeMiss,
						metadata.AttributeOperationGet)
				}
			case "incr_hits":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedOperationsDataPoint(now, parsedV, metadata.AttributeTypeHit,
						metadata.AttributeOperationIncrement)
				}
			case "incr_misses":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedOperationsDataPoint(now, parsedV, metadata.AttributeTypeMiss,
						metadata.AttributeOperationIncrement)
				}
			case "decr_hits":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedOperationsDataPoint(now, parsedV, metadata.AttributeTypeHit,
						metadata.AttributeOperationDecrement)
				}
			case "decr_misses":
				if parsedV, ok := r.parseInt(k, v); ok {
					r.mb.RecordMemcachedOperationsDataPoint(now, parsedV, metadata.AttributeTypeMiss,
						metadata.AttributeOperationDecrement)
				}
			case "rusage_system":
				if parsedV, ok := r.parseFloat(k, v); ok {
					r.mb.RecordMemcachedCPUUsageDataPoint(now, parsedV, metadata.AttributeStateSystem)
				}

			case "rusage_user":
				if parsedV, ok := r.parseFloat(k, v); ok {
					r.mb.RecordMemcachedCPUUsageDataPoint(now, parsedV, metadata.AttributeStateUser)
				}
			}
		}

		// Calculated Metrics
		parsedHit, okHit := r.parseInt("incr_hits", stats.Stats["incr_hits"])
		parsedMiss, okMiss := r.parseInt("incr_misses", stats.Stats["incr_misses"])
		if okHit && okMiss {
			r.mb.RecordMemcachedOperationHitRatioDataPoint(now, calculateHitRatio(parsedHit, parsedMiss),
				metadata.AttributeOperationIncrement)
		}

		parsedHit, okHit = r.parseInt("decr_hits", stats.Stats["decr_hits"])
		parsedMiss, okMiss = r.parseInt("decr_misses", stats.Stats["decr_misses"])
		if okHit && okMiss {
			r.mb.RecordMemcachedOperationHitRatioDataPoint(now, calculateHitRatio(parsedHit, parsedMiss),
				metadata.AttributeOperationDecrement)
		}

		parsedHit, okHit = r.parseInt("get_hits", stats.Stats["get_hits"])
		parsedMiss, okMiss = r.parseInt("get_misses", stats.Stats["get_misses"])
		if okHit && okMiss {
			r.mb.RecordMemcachedOperationHitRatioDataPoint(now, calculateHitRatio(parsedHit, parsedMiss), metadata.AttributeOperationGet)
		}
	}

	return r.mb.Emit(), nil
}

func calculateHitRatio(misses, hits int64) float64 {
	if misses+hits == 0 {
		return 0
	}
	hitsFloat := float64(hits)
	missesFloat := float64(misses)
	return hitsFloat / (hitsFloat + missesFloat) * 100
}

// parseInt converts string to int64.
func (r *memcachedScraper) parseInt(key, value string) (int64, bool) {
	i, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		r.logInvalid("int", key, value)
		return 0, false
	}
	return i, true
}

// parseFloat converts string to float64.
func (r *memcachedScraper) parseFloat(key, value string) (float64, bool) {
	i, err := strconv.ParseFloat(value, 64)
	if err != nil {
		r.logInvalid("float", key, value)
		return 0, false
	}
	return i, true
}

func (r *memcachedScraper) logInvalid(expectedType, key, value string) {
	r.logger.Info(
		"invalid value",
		zap.String("expectedType", expectedType),
		zap.String("key", key),
		zap.String("value", value),
	)
}
