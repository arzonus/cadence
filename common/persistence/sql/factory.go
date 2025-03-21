// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package sql

import (
	"fmt"
	"sync"

	"github.com/uber/cadence/common/config"
	"github.com/uber/cadence/common/log"
	p "github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/serialization"
	"github.com/uber/cadence/common/persistence/sql/sqlplugin"
)

type (
	// Factory vends store objects backed by MySQL
	Factory struct {
		cfg            config.SQL
		dbConn         dbConn
		clusterName    string
		logger         log.Logger
		parser         serialization.Parser
		taskSerializer serialization.TaskSerializer
		dc             *p.DynamicConfiguration
	}

	// dbConn represents a logical mysql connection - its a
	// wrapper around the standard sql connection pool with
	// additional reference counting
	dbConn struct {
		sync.Mutex
		sqlplugin.DB
		refCnt int
		cfg    *config.SQL
	}
)

// NewFactory returns an instance of a factory object which can be used to create
// datastores backed by any kind of SQL store
func NewFactory(
	cfg config.SQL,
	clusterName string,
	logger log.Logger,
	parser serialization.Parser,
	dc *p.DynamicConfiguration,
) *Factory {
	return &Factory{
		cfg:            cfg,
		clusterName:    clusterName,
		logger:         logger,
		dbConn:         newRefCountedDBConn(&cfg),
		parser:         parser,
		taskSerializer: serialization.NewTaskSerializer(parser),
		dc:             dc,
	}
}

// NewTaskStore returns a new task store
func (f *Factory) NewTaskStore() (p.TaskStore, error) {
	conn, err := f.dbConn.get()
	if err != nil {
		return nil, err
	}
	return newTaskPersistence(conn, f.cfg.NumShards, f.logger, f.parser)
}

// NewShardStore returns a new shard store
func (f *Factory) NewShardStore() (p.ShardStore, error) {
	conn, err := f.dbConn.get()
	if err != nil {
		return nil, err
	}
	return NewShardPersistence(conn, f.clusterName, f.logger, f.parser)
}

// NewHistoryStore returns a new history store
func (f *Factory) NewHistoryStore() (p.HistoryStore, error) {
	conn, err := f.dbConn.get()
	if err != nil {
		return nil, err
	}
	return NewHistoryV2Persistence(conn, f.logger, f.parser)
}

// NewDomainStore returns a new metadata store
func (f *Factory) NewDomainStore() (p.DomainStore, error) {
	conn, err := f.dbConn.get()
	if err != nil {
		return nil, err
	}
	return newMetadataPersistenceV2(conn, f.clusterName, f.logger, f.parser)
}

// NewExecutionStore returns an ExecutionStore for a given shardID
func (f *Factory) NewExecutionStore(shardID int) (p.ExecutionStore, error) {
	conn, err := f.dbConn.get()
	if err != nil {
		return nil, err
	}
	return NewSQLExecutionStore(conn, f.logger, shardID, f.parser, f.taskSerializer, f.dc)
}

// NewVisibilityStore returns a visibility store
// TODO sortByCloseTime will be removed and implemented for https://github.com/uber/cadence/issues/3621
func (f *Factory) NewVisibilityStore(sortByCloseTime bool) (p.VisibilityStore, error) {
	return NewSQLVisibilityStore(f.cfg, f.logger)
}

// NewQueue returns a new queue backed by sql
func (f *Factory) NewQueue(queueType p.QueueType) (p.Queue, error) {
	conn, err := f.dbConn.get()
	if err != nil {
		return nil, err
	}

	return newQueueStore(conn, f.logger, queueType)
}

// NewConfigStore returns a new config store backed by sql. Not Yet Implemented.
func (f *Factory) NewConfigStore() (p.ConfigStore, error) {
	conn, err := f.dbConn.get()
	if err != nil {
		return nil, err
	}
	return NewSQLConfigStore(conn, f.logger, f.parser)
}

// Close closes the factory
func (f *Factory) Close() {
	f.dbConn.forceClose()
}

// newRefCountedDBConn returns a  logical mysql connection that
// uses reference counting to decide when to close the
// underlying connection object. The reference count gets incremented
// everytime get() is called and decremented everytime Close() is called
func newRefCountedDBConn(cfg *config.SQL) dbConn {
	return dbConn{cfg: cfg}
}

// get returns a mysql db connection and increments a reference count
// this method will create a new connection, if an existing connection
// does not exist
func (c *dbConn) get() (sqlplugin.DB, error) {
	c.Lock()
	defer c.Unlock()
	if c.refCnt == 0 {
		conn, err := NewSQLDB(c.cfg)
		if err != nil {
			return nil, err
		}
		c.DB = conn
	}
	c.refCnt++
	return c, nil
}

// forceClose ignores reference counts and shutsdown the underlying connection pool
func (c *dbConn) forceClose() {
	c.Lock()
	defer c.Unlock()
	if c.DB != nil {
		err := c.DB.Close()
		if err != nil {
			fmt.Println("failed to close database connection, may leak some connection", err)
		}

	}
	c.refCnt = 0
}

// Close closes the underlying connection if the reference count becomes zero
func (c *dbConn) Close() error {
	c.Lock()
	defer c.Unlock()
	c.refCnt--
	if c.refCnt == 0 {
		err := c.DB.Close()
		c.DB = nil
		return err
	}
	return nil
}
