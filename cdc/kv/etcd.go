// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package kv

import (
	"context"
	"fmt"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/pingcap/errors"
	"github.com/pingcap/ticdc/cdc/model"
	"github.com/pingcap/ticdc/pkg/util"
)

const (
	// EtcdKeyBase is the common prefix of the keys in CDC
	EtcdKeyBase = "/tidb/cdc"
)

// GetEtcdKeyChangeFeedList returns the prefix key of all changefeed config
func GetEtcdKeyChangeFeedList() string {
	return fmt.Sprintf("%s/changefeed/config", EtcdKeyBase)
}

// GetEtcdKeyChangeFeedConfig returns the key of a changefeed config
func GetEtcdKeyChangeFeedConfig(changefeedID string) string {
	return fmt.Sprintf("%s/%s", GetEtcdKeyChangeFeedList(), changefeedID)
}

// GetEtcdKeyChangeFeedStatus returns the key of a changefeed status
func GetEtcdKeyChangeFeedStatus(changefeedID string) string {
	return fmt.Sprintf("%s/changefeed/status/%s", EtcdKeyBase, changefeedID)
}

// GetEtcdKeySubChangeFeedList returns the key of a subchangefeed info without captureID part
func GetEtcdKeySubChangeFeedList(changefeedID string) string {
	return fmt.Sprintf("%s/changefeed/subchangfeed/%s", EtcdKeyBase, changefeedID)
}

// GetEtcdKeySubChangeFeed returns the key of a subchangefeed infoformation
func GetEtcdKeySubChangeFeed(changefeedID, captureID string) string {
	return fmt.Sprintf("%s/%s", GetEtcdKeySubChangeFeedList(changefeedID), captureID)
}

// GetEtcdKeyCaptureList returns the prefix key of all capture info
func GetEtcdKeyCaptureList() string {
	return EtcdKeyBase + "/capture/info"
}

// GetChangeFeeds returns kv revision and a map mapping from changefeedID to changefeed detail mvccpb.KeyValue
func GetChangeFeeds(ctx context.Context, cli *clientv3.Client, opts ...clientv3.OpOption) (int64, map[string]*mvccpb.KeyValue, error) {
	key := GetEtcdKeyChangeFeedList()

	resp, err := cli.Get(ctx, key, clientv3.WithPrefix())
	if err != nil {
		return 0, nil, errors.Trace(err)
	}
	revision := resp.Header.Revision
	details := make(map[string]*mvccpb.KeyValue, resp.Count)
	for _, kv := range resp.Kvs {
		id, err := util.ExtractKeySuffix(string(kv.Key))
		if err != nil {
			return 0, nil, err
		}
		details[id] = kv
	}
	return revision, details, nil
}

// GetChangeFeedDetail queries the config of a given changefeed
func GetChangeFeedDetail(ctx context.Context, cli *clientv3.Client, id string, opts ...clientv3.OpOption) (*model.ChangeFeedDetail, error) {
	key := GetEtcdKeyChangeFeedConfig(id)
	resp, err := cli.Get(ctx, key, opts...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if resp.Count == 0 {
		return nil, errors.Annotatef(model.ErrChangeFeedNotExists, "query detail id %s", id)
	}
	detail := &model.ChangeFeedDetail{}
	err = detail.Unmarshal(resp.Kvs[0].Value)
	return detail, errors.Trace(err)
}

// GetChangeFeedInfo queries the checkpointTs and resovledTs of a given changefeed
func GetChangeFeedInfo(ctx context.Context, cli *clientv3.Client, id string, opts ...clientv3.OpOption) (*model.ChangeFeedInfo, error) {
	key := GetEtcdKeyChangeFeedStatus(id)
	resp, err := cli.Get(ctx, key, opts...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if resp.Count == 0 {
		return nil, errors.Annotatef(model.ErrChangeFeedNotExists, "query status id %s", id)
	}
	info := &model.ChangeFeedInfo{}
	err = info.Unmarshal(resp.Kvs[0].Value)
	return info, errors.Trace(err)
}

// GetCaptures returns kv revision and CaptureInfo list
func GetCaptures(ctx context.Context, cli *clientv3.Client, opts ...clientv3.OpOption) (int64, []*model.CaptureInfo, error) {
	key := GetEtcdKeyCaptureList()

	resp, err := cli.Get(ctx, key, clientv3.WithPrefix())
	if err != nil {
		return 0, nil, errors.Trace(err)
	}
	revision := resp.Header.Revision
	infos := make([]*model.CaptureInfo, 0, resp.Count)
	for _, kv := range resp.Kvs {
		info := &model.CaptureInfo{}
		err := info.Unmarshal(kv.Value)
		if err != nil {
			return 0, nil, errors.Trace(err)
		}
		infos = append(infos, info)
	}
	return revision, infos, nil
}

// SaveChangeFeedDetail stores change feed detail into etcd
// TODO: this should be called from outer system, such as from a TiDB client
func SaveChangeFeedDetail(ctx context.Context, client *clientv3.Client, detail *model.ChangeFeedDetail, changeFeedID string) error {
	key := GetEtcdKeyChangeFeedConfig(changeFeedID)
	value, err := detail.Marshal()
	if err != nil {
		return errors.Trace(err)
	}
	_, err = client.Put(ctx, key, value)
	return errors.Trace(err)
}

// GetSubChangeFeedInfos queries all subchangefeed info of a changefeed, and returns a map
// mapping from captureID to SubChangeFeedInfo
func GetSubChangeFeedInfos(ctx context.Context, client *clientv3.Client, changefeedID string, opts ...clientv3.OpOption) (model.ProcessorsInfos, error) {
	key := GetEtcdKeySubChangeFeedList(changefeedID)
	resp, err := client.Get(ctx, key, append([]clientv3.OpOption{clientv3.WithPrefix()}, opts...)...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	pinfo := make(map[string]*model.SubChangeFeedInfo, resp.Count)
	for _, rawKv := range resp.Kvs {
		captureID, err := util.ExtractKeySuffix(string(rawKv.Key))
		if err != nil {
			return nil, err
		}
		info := &model.SubChangeFeedInfo{}
		err = info.Unmarshal(rawKv.Value)
		if err != nil {
			return nil, err
		}
		info.ModRevision = rawKv.ModRevision
		pinfo[captureID] = info
	}
	return pinfo, nil
}

// GetSubChangeFeedInfo queries subchangefeed info from etcd, returns
//  - ModRevision of the given key
//  - *model.SubChangeFeedInfo unmarshaled from the value
//  - error if error happens
func GetSubChangeFeedInfo(
	ctx context.Context,
	client *clientv3.Client,
	changefeedID string,
	captureID string,
	opts ...clientv3.OpOption,
) (int64, *model.SubChangeFeedInfo, error) {
	key := GetEtcdKeySubChangeFeed(changefeedID, captureID)
	resp, err := client.Get(ctx, key, opts...)
	if err != nil {
		return 0, nil, errors.Trace(err)
	}
	if resp.Count == 0 {
		return 0, nil, errors.Annotatef(model.ErrSubChangeFeedInfoNotExists, "changefeed: %s, capture: %s", changefeedID, captureID)
	}
	info := &model.SubChangeFeedInfo{}
	err = info.Unmarshal(resp.Kvs[0].Value)
	return resp.Kvs[0].ModRevision, info, errors.Trace(err)
}

// PutSubChangeFeedInfo puts subchangefeed info into etcd.
func PutSubChangeFeedInfo(
	ctx context.Context,
	client *clientv3.Client,
	changefeedID string,
	captureID string,
	info *model.SubChangeFeedInfo,
	opts ...clientv3.OpOption,
) error {
	data, err := info.Marshal()
	if err != nil {
		return errors.Trace(err)
	}

	key := GetEtcdKeySubChangeFeed(changefeedID, captureID)

	_, err = client.Put(ctx, key, data)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// PutChangeFeedStatus puts changefeed synchronization status into etcd
func PutChangeFeedStatus(
	ctx context.Context,
	client *clientv3.Client,
	changefeedID string,
	info *model.ChangeFeedInfo,
	opts ...clientv3.OpOption,
) error {
	key := GetEtcdKeyChangeFeedStatus(changefeedID)
	value, err := info.Marshal()
	if err != nil {
		return err
	}
	_, err = client.Put(ctx, key, value, opts...)
	return errors.Trace(err)
}
