package db

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/CSUNetSec/bgpmon/v2/config"
	"github.com/CSUNetSec/bgpmon/v2/util"
	"github.com/sirupsen/logrus"
	"time"
)

const (
	POSTGRES = iota
)

const (
	CONNECT_NO_SSL       = "connectNoSSL"
	CONNECT_SSL          = "connectSSL"
	CHECK_SCHEMA         = "checkschema"
	SELECT_NODE          = "selectNodeTmpl"
	INSERT_NODE          = "insertNodeTmpl"
	INSERT_MAIN_TABLE    = "insertMainTableTmpl"
	MAKE_MAIN_TABLE      = "makeMainTableTmpl"
	SELECT_TABLE         = "selectTableTmpl"
	MAKE_NODE_TABLE      = "makeNodeTableTmpl"
	MAKE_CAPTURE_TABLE   = "makeCaptureTableTmpl"
	INSERT_CAPTURE_TABLE = "insertCaptureTableTmpl"
)

var dbops = map[string][]string{
	CONNECT_NO_SSL: []string{
		//postgress
		`user=%s password=%s dbname=%s host=%s sslmode=disable`,
	},
	CONNECT_SSL: []string{
		//postgress
		`user=%s password=%s dbname=%s host=%s`,
	},
	CHECK_SCHEMA: []string{
		//postgress
		`SELECT EXISTS (
		   SELECT *
		   FROM   information_schema.tables
		   WHERE  table_name = $1
		 );`,
	},
	SELECT_NODE: []string{
		//postgress
		`SELECT name, ip, isCollector, tableDumpDurationMinutes, description, coords, address FROM %s;`,
	},
	INSERT_NODE: []string{
		//postgress
		`INSERT INTO %s (name, ip, isCollector, tableDumpDurationMinutes, description, coords, address) 
		   VALUES ($1, $2, $3, $4, $5, $6, $7)
		   ON CONFLICT (ip) DO UPDATE SET name=EXCLUDED.name, isCollector=EXCLUDED.isCollector, 
		     tableDumpDurationMinutes=EXCLUDED.tableDumpDurationMinutes,
		     description=EXCLUDED.description, coords=EXCLUDED.coords, address=EXCLUDED.address;`,
	},
	MAKE_MAIN_TABLE: []string{
		//postgress
		`CREATE TABLE IF NOT EXISTS %s (
		   dbname varchar PRIMARY KEY,
	           collector varchar,
	           dateFrom timestamp,
	           dateTo timestamp
                 );`,
	},
	INSERT_MAIN_TABLE: []string{
		//postgress
		`INSERT INTO %s (dbname, collector, dateFrom, dateTo) VALUES ($1, $2, $3, $4);`,
	},
	MAKE_CAPTURE_TABLE: []string{
		//postgress
		`CREATE TABLE IF NOT EXISTS %s (
		   update_id varchar PRIMARY KEY, timestamp timestamp, collector_ip inet, peer_ip inet, as_path integer[], next_hop inet, origin_as integer, update_withdraw bool, protomsg bytea);`,
	},
	INSERT_CAPTURE_TABLE: []string{
		`INSERT INTO %s (update_id, timestamp, collector_ip, peer_ip, as_path, next_hop, origin_as, update_withdraw, protomsg)`,
	},
	SELECT_TABLE: []string{
		//postgress
		`SELECT dbname, collector, dateFrom, dateTo FROM %s 
		 WHERE dateFrom <= $1 AND dateTo > $1;`,
	},
	MAKE_NODE_TABLE: []string{
		//postgress
		`CREATE TABLE IF NOT EXISTS %s (
		   ip varchar PRIMARY KEY,
		   name varchar, 
		   isCollector boolean,
		   tableDumpDurationMinutes integer,
		   description varchar,
		   coords varchar,
		   address varchar
	         );`,
	},
}

var (
	dblogger = logrus.WithField("system", "db")
)

type Dber interface {
	Db() *sql.DB
}

//a struct for issuing queries about the existance of a ready collector table
//for a specific time. Typically on the return we will return the starting
//time for that table as a string so that the caller can just concat and create
//the destination table names
type collectorDate struct {
	col    string    //the collector we are querying for
	dat    time.Time //the time we are interested
	datstr string    //the time string returned that will create the table name
}

func newCollectorDate(col string, t time.Time) collectorDate {
	return collectorDate{
		col: col,
		dat: t,
	}
}

//a wrapper struct that can contain all the possible arguments to our database calls
type sqlIn struct {
	dbname        string                       //the name of the database we're operating on
	maintable     string                       //the table which references all collector-day tables.
	nodetable     string                       //the table with nodes and their configurations
	knownNodes    map[string]config.NodeConfig //an incoming map of the known nodes
	getNodeName   string                       //a node name we want to fetch is config from the db
	getNodeIP     string                       //a node IP we want to fetch is config from the db
	getColDate    collectorDate                //a collector name and a date that we want to write messages for
	capTableName  string                       //the name of a capture table in the form of nodename-startdate
	capTableCol   string                       //the ip of the capture table collector
	capTableSdate time.Time                    //the start date of the capture table
	capTableEdate time.Time                    //the end date of the capture table
}

type sqlOut struct {
	ok            bool
	err           error
	knownNodes    map[string]config.NodeConfig //a composition of the incoming and already known nodes
	resultNode    *node                        //the result from a getNode call
	resultColDate collectorDate                //the results of a collectorDate query
	capTable      string                       //the name of the capture table for that message
	capIp         string                       //the ip of the collector for this capture table
	capStime      time.Time                    //start time for the capture table
	capEtime      time.Time                    //end time for the capture table
}

type getdboper interface {
	getdbop(string) string
}

type dbOper struct {
	t int
}

// Gets the specific db op string from the static table declared in db.go
// for the appropriate dbType that was populated when the correct newSession was called.
// Panics on error.
// implementing the getdboper interface in db.go
func (d *dbOper) getdbop(a string) (ret string) {
	if sslice, exists := dbops[a]; !exists {
		panic(fmt.Sprintf("nx db op name:%s requested.", a))
	} else if len(sslice)-1 < d.t {
		panic(fmt.Sprintf("dbop:%s for this db type not populated", a))
	} else {
		ret = sslice[d.t]
	}
	return
}

func newPostgressDbOper() *dbOper {
	return &dbOper{
		t: POSTGRES,
	}
}

type SessionExecutor interface {
	util.SqlExecutor
	getdboper
}

type ctxtxOperExecutor struct {
	*ctxTx
	*dbOper
}

func newCtxTxSessionExecutor(cex *ctxTx, dbo *dbOper) *ctxtxOperExecutor {
	return &ctxtxOperExecutor{
		cex,
		dbo,
	}
}

type dbOperExecutor struct {
	*sql.DB
	*dbOper
}

func newDbSessionExecutor(db *sql.DB, dbo *dbOper) *dbOperExecutor {
	return &dbOperExecutor{
		db,
		dbo,
	}
}

//creates a new ctxTx for that operation which implements the
//sqlExecutor interface. The argument passed instructs it to either
//do it on a transaction if true, or on the normal DB connection if false.
//caller must call Done() that releases resources.
func GetNewExecutor(pc context.Context, s Dber, doTx bool, ctxTimeout time.Duration) (*ctxTx, error) {
	var (
		tx  *sql.Tx
		err error
		db  *sql.DB
	)
	db = s.Db()
	ctx, cf := context.WithTimeout(pc, ctxTimeout)
	if doTx {
		if tx, err = db.BeginTx(ctx, nil); err != nil {
			cf()
			return nil, err
		}
	} else {
		tx = nil
	}
	return &ctxTx{
		doTx: doTx,
		tx:   tx,
		cf:   cf,
		ctx:  ctx,
		db:   db,
	}, nil
}

func (c *ctxTx) Exec(query string, args ...interface{}) (sql.Result, error) {
	if c.doTx && c.tx != nil {
		return c.tx.ExecContext(c.ctx, query, args...)
	}
	return c.db.ExecContext(c.ctx, query, args...)
}

func (c *ctxTx) Query(query string, args ...interface{}) (*sql.Rows, error) {
	if c.doTx && c.tx != nil {
		return c.tx.QueryContext(c.ctx, query, args...)
	}
	return c.db.QueryContext(c.ctx, query, args...)
}

func (c *ctxTx) QueryRow(query string, args ...interface{}) *sql.Row {
	if c.doTx && c.tx != nil {
		return c.tx.QueryRowContext(c.ctx, query, args...)
	}
	return c.db.QueryRowContext(c.ctx, query, args...)
}

//a wrapper of a sql.Tx that is able to accept multiple
//db ops and run them in the same tx.
//it will implement the SqlExectutor interface and choose
//where to apply the sql function depending on how it was constructed.
//(either apply everything in the transaction and then the last Done()
//will commit, or straight on the DB and the last Done() is a noop.
//the ctxTx structs are created by the specific sessions.
type ctxTx struct {
	doTx bool
	tx   *sql.Tx
	db   *sql.DB
	cf   context.CancelFunc
	ctx  context.Context
}

//either commits the TX or just releases the context through it's cancelfunc.
func (ptx *ctxTx) Done() error {
	defer ptx.cf() //release resources if it's done.
	if ptx.doTx && ptx.tx != nil {
		return ptx.tx.Commit()
	}
	return nil
}

//This is a representation of a node that is stored in the database using this fields.
//a node can be either a collector or a peer, and in case of being a collector it is used
//to generate the table names that data collected by it are stored. it should be also geolocated.
//known nodes can be supplied by the config file.
type node struct {
	nodeName      string
	nodeIP        string
	nodeCollector bool
	nodeDuration  int
	nodeDescr     string
	nodeCoords    string
	nodeAddress   string
}

//creates an empty node
func newNode() *node {
	return &node{}
}

//creates a nodeconfig from a node
func (a *node) nodeConfigFromNode() config.NodeConfig {
	return config.NodeConfig{
		Name:                a.nodeName,
		IP:                  a.nodeIP,
		IsCollector:         a.nodeCollector,
		DumpDurationMinutes: a.nodeDuration,
		Description:         a.nodeDescr,
		Coords:              a.nodeCoords,
		Location:            a.nodeAddress,
	}
}
