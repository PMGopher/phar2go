package phpx

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// DbConn is a database connection for phar2go's versions of PHP's SQLite3 and mysqli classes,
// on Go's database/sql (the drivers are imported by the converted plugin).
type DbConn struct {
	db           *sql.DB
	conn         *sql.Conn
	tx           *sql.Tx
	changes      int
	lastInsertID int
	errMsg       string
	errCode      int
}

// MYSQLI_USE_RESULT is mysqli::query()'s unbuffered mode.
const MYSQLI_USE_RESULT = 1

// Phar2goDbOpen opens a database: driver "sqlite" (dsn is the file) or "mysql" (dsn is
// "user:password@tcp(host:port)/database"). It returns the connection, or the error message.
func Phar2goDbOpen(driver string, dsn string) any {
	if driver == "mysql" && !strings.Contains(dsn, "?") {
		dsn += "?parseTime=false&multiStatements=true"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return err.Error()
	}
	if driver == "sqlite" {
		db.SetMaxOpenConns(1)
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		db.Close()
		return err.Error()
	}
	if err := conn.PingContext(context.Background()); err != nil {
		conn.Close()
		db.Close()
		return err.Error()
	}
	return &DbConn{db: db, conn: conn}
}

// Phar2goDbMysqlDsn is the database/sql DSN of mysqli's connection arguments.
func Phar2goDbMysqlDsn(host, user, password, database string, port int) string {
	if host == "" || host == "localhost" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 3306
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", user, password, host, port, database)
}

type dbExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (c *DbConn) target() dbExecer {
	if c.tx != nil {
		return c.tx
	}
	return c.conn
}

func dbArgs(params any) []any {
	a := ToArray(params)
	var out []any
	for _, e := range a.Entries() {
		v := e.Val
		if b, ok := v.(bool); ok {
			if b {
				v = 1
			} else {
				v = 0
			}
		}
		if k, ok := e.Key.(string); ok {
			out = append(out, sql.Named(strings.TrimLeft(k, ":@$"), v))
		} else {
			out = append(out, v)
		}
	}
	return out
}

func (c *DbConn) fail(err error) bool {
	c.errMsg = err.Error()
	c.errCode = 1
	return false
}

// Phar2goDbExec runs a statement that returns no rows; false on error.
func Phar2goDbExec(conn any, query string, params any) bool {
	c := conn.(*DbConn)
	c.errMsg, c.errCode = "", 0
	res, err := c.target().ExecContext(context.Background(), query, dbArgs(params)...)
	if err != nil {
		return c.fail(err)
	}
	if n, err := res.RowsAffected(); err == nil {
		c.changes = int(n)
	}
	if id, err := res.LastInsertId(); err == nil && id != 0 {
		c.lastInsertID = int(id)
	}
	return true
}

// Phar2goDbQuery runs a query: [column names, rows (lists of values)], or false on error.
func Phar2goDbQuery(conn any, query string, params any) any {
	c := conn.(*DbConn)
	c.errMsg, c.errCode = "", 0
	rows, err := c.target().QueryContext(context.Background(), query, dbArgs(params)...)
	if err != nil {
		return c.fail(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	names := NewArray()
	for _, n := range cols {
		names.Append(n)
	}
	out := NewArray()
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return c.fail(err)
		}
		row := NewArray()
		for _, v := range vals {
			row.Append(sqlToPHP(v))
		}
		out.Append(row)
	}
	if err := rows.Err(); err != nil {
		return c.fail(err)
	}
	return List(names, out)
}

// Phar2goDbIsQuery reports whether a statement returns rows (SELECT, PRAGMA, ...).
func Phar2goDbIsQuery(query string) bool {
	q := strings.ToUpper(strings.TrimLeft(query, " \t\r\n("))
	for _, p := range []string{"SELECT", "PRAGMA", "WITH", "SHOW", "DESCRIBE", "EXPLAIN", "VALUES"} {
		if strings.HasPrefix(q, p) {
			return true
		}
	}
	return strings.Contains(q, " RETURNING ")
}

// Phar2goDbBegin starts a transaction.
func Phar2goDbBegin(conn any) bool {
	c := conn.(*DbConn)
	if c.tx != nil {
		return false
	}
	tx, err := c.conn.BeginTx(context.Background(), nil)
	if err != nil {
		return c.fail(err)
	}
	c.tx = tx
	return true
}

// Phar2goDbCommit commits the transaction.
func Phar2goDbCommit(conn any) bool {
	c := conn.(*DbConn)
	if c.tx == nil {
		return false
	}
	err := c.tx.Commit()
	c.tx = nil
	if err != nil {
		return c.fail(err)
	}
	return true
}

// Phar2goDbRollback rolls the transaction back.
func Phar2goDbRollback(conn any) bool {
	c := conn.(*DbConn)
	if c.tx == nil {
		return false
	}
	err := c.tx.Rollback()
	c.tx = nil
	if err != nil {
		return c.fail(err)
	}
	return true
}

// Phar2goDbChanges is the number of rows the last statement changed.
func Phar2goDbChanges(conn any) int { return conn.(*DbConn).changes }

// Phar2goDbLastInsertId is the id of the last inserted row.
func Phar2goDbLastInsertId(conn any) int { return conn.(*DbConn).lastInsertID }

// Phar2goDbError is the error message of the last statement ("" if it succeeded).
func Phar2goDbError(conn any) string { return conn.(*DbConn).errMsg }

// Phar2goDbErrorCode is the error code of the last statement (0 if it succeeded).
func Phar2goDbErrorCode(conn any) int { return conn.(*DbConn).errCode }

// Phar2goDbClose closes the connection.
func Phar2goDbClose(conn any) bool {
	c, ok := conn.(*DbConn)
	if !ok {
		return false
	}
	if c.tx != nil {
		c.tx.Rollback()
		c.tx = nil
	}
	c.conn.Close()
	return c.db.Close() == nil
}
