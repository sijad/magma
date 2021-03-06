/*
 * Copyright (c) Facebook, Inc. and its affiliates.
 * All rights reserved.
 *
 * This source code is licensed under the BSD-style license found in the
 * LICENSE file in the root directory of this source tree.
 */

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"magma/orc8r/cloud/go/obsidian"

	"github.com/golang/glog"
	"github.com/labstack/echo"
	"github.com/olivere/elastic/v7"
)

const (
	Events     = "events"
	EventsPath = obsidian.V1Root +
		Events + obsidian.UrlSep +
		":" + pathParamNetworkID + obsidian.UrlSep +
		":" + pathParamStreamName

	pathParamStreamName  = "stream_name"
	pathParamNetworkID   = "network_id"
	queryParamEventType  = "event_type"
	queryParamHardwareID = "hardware_id"
	queryParamTag        = "tag"

	defaultQuerySize = 50
	timestamp        = "timestamp"

	// We use the ES "keyword" type for exact match
	dotKeyword              = ".keyword"
	elasticFilterStreamName = pathParamStreamName + dotKeyword
	elasticFilterNetworkID  = pathParamNetworkID + dotKeyword
	elasticFilterEventType  = queryParamEventType + dotKeyword
	elasticFilterHardwareID = "hw_id" + dotKeyword
	elasticFilterEventTag   = "event_tag" + dotKeyword // We use event_tag as fluentd uses the "tag" field
)

// GetEventsHandler returns a Hander that uses the provided elastic client
func GetEventsHandler(client *elastic.Client) func(c echo.Context) error {
	return func(c echo.Context) error {
		return EventsHandler(c, client)
	}
}

// EventsHandler handles event querying using ES
func EventsHandler(c echo.Context, client *elastic.Client) error {
	queryParams, err := getQueryParameters(c)
	if err != nil {
		return obsidian.HttpError(err, http.StatusBadRequest)
	}

	elasticQuery := queryParams.ToElasticBoolQuery()

	result, err := client.Search().
		Index("").
		Size(defaultQuerySize).
		Query(elasticQuery).
		Do(context.Background())
	if err != nil {
		glog.Errorf("Error getting response from Elastic: %s", err)
		return obsidian.HttpError(err, http.StatusInternalServerError)
	}
	if result.Error != nil {
		return obsidian.HttpError(fmt.Errorf(
			"Elastic Error Type: %s, Reason: %s",
			result.Error.Type,
			result.Error.Reason))
	}

	maps, err := getEventMaps(result.Hits.Hits)
	if err != nil {
		glog.Error(err)
		return obsidian.HttpError(err, http.StatusInternalServerError)
	}

	return c.JSON(http.StatusOK, maps)
}

type eventResult struct {
	StreamName string `json:"stream_name"`
	EventType  string `json:"event_type"`
	// FluentBit logs sent from AGW are tagged with hw_id
	HardwareID string `json:"hw_id"`
	// We use event_tag as fluentd uses the "tag" field
	Tag       string `json:"event_tag"`
	Timestamp string `json:"@timestamp"`
	Value     string `json:"value"`
}

// Retrieve Event properties from the _source of
// ES Hits, including event metadata
func getEventMaps(hits []*elastic.SearchHit) ([]map[string]interface{}, error) {
	results := []map[string]interface{}{}
	for _, hit := range hits {
		var result eventResult
		// Get Value from the _source
		if err := json.Unmarshal(hit.Source, &result); err != nil {
			return nil, fmt.Errorf("Unable to Unmarshal JSON from elastic.Hit. "+
				"elastic.Hit.Source: %s, Error: %s", hit.Source, err)
		}
		// Skip hits without an event value
		if result.Value == "" {
			return nil, fmt.Errorf("eventResult %s does not contain a value", result)
		}
		// Get event metadata
		mapToAdd := map[string]interface{}{
			pathParamStreamName:  result.StreamName,
			queryParamEventType:  result.EventType,
			queryParamHardwareID: result.HardwareID,
			queryParamTag:        result.Tag,
			timestamp:            result.Timestamp,
		}
		// Get event value fields
		var eventValueMap map[string]interface{}
		if err := json.Unmarshal([]byte(result.Value), &eventValueMap); err != nil {
			return nil, fmt.Errorf("Unable to Unmarshal JSON from eventResult.Value. "+
				"eventResult.Value: %s, Error: %s", hit.Source, err)
		}
		for k, v := range eventValueMap {
			mapToAdd[k] = v
		}
		results = append(results, mapToAdd)
	}
	return results, nil
}

func getQueryParameters(c echo.Context) (eventQueryParams, error) {
	streamName := c.Param(pathParamStreamName)
	if streamName == "" {
		return eventQueryParams{}, StreamNameHTTPErr()
	}
	networkID, nerr := obsidian.GetNetworkId(c)
	if nerr != nil {
		return eventQueryParams{}, nerr
	}
	params := eventQueryParams{
		StreamName: streamName,
		EventType:  c.QueryParam(queryParamEventType),
		HardwareID: c.QueryParam(queryParamHardwareID),
		NetworkID:  networkID,
		Tag:        c.QueryParam(queryParamTag),
	}
	return params, nil
}

type eventQueryParams struct {
	StreamName string
	EventType  string
	HardwareID string
	NetworkID  string
	Tag        string
}

func (b *eventQueryParams) ToElasticBoolQuery() *elastic.BoolQuery {
	query := elastic.NewBoolQuery()
	query.Filter(elastic.NewTermQuery(elasticFilterStreamName, b.StreamName))
	query.Filter(elastic.NewTermQuery(elasticFilterNetworkID, b.NetworkID))
	if len(b.EventType) > 0 {
		query.Filter(elastic.NewTermQuery(elasticFilterEventType, b.EventType))
	}
	if len(b.HardwareID) > 0 {
		query.Filter(elastic.NewTermQuery(elasticFilterHardwareID, b.HardwareID))
	}
	if len(b.Tag) > 0 {
		query.Filter(elastic.NewTermQuery(elasticFilterEventTag, b.Tag))
	}
	return query
}

// StreamNameHTTPErr indicates that stream_name is missing
func StreamNameHTTPErr() *echo.HTTPError {
	return obsidian.HttpError(fmt.Errorf("Missing stream name"), http.StatusBadRequest)
}
