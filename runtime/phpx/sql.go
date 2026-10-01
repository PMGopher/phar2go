package phpx

import (
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SqlConn is a database connection of the libasynql replacement: libasynql's queries (from
// its .sql "prepared statement files") run on database/sql. The drivers are imported by the
// converted plugin (modernc.org/sqlite and github.com/go-sql-driver/mysql).
type SqlConn struct {
	db      *sql.DB
	dialect string
	queries map[string]*sqlQuery
	logging bool
}

type sqlVar struct {
	name     string
	typ      string
	list     bool
	canEmpty bool
	nullable bool
	def      any
	hasDef   bool
}

type sqlQuery struct {
	name  string
	parts []string
	vars  map[string]*sqlVar
}

// Phar2goSqlOpen opens the database described by libasynql's config ("type", "sqlite" =>
// ["file"], "mysql" => ["host", "username", "password", "schema", "port"]).
func Phar2goSqlOpen(config any, dataFolder string) any {
	conf := ToArray(FromGo(config))
	dialect := strings.ToLower(ToString(conf.Get("type")))
	var db *sql.DB
	var err error
	switch dialect {
	case "sqlite", "sqlite3", "sq3":
		dialect = "sqlite"
		file := ToString(Index(conf.Get("sqlite"), "file"))
		if file == "" {
			file = "data.sqlite"
		}
		if !filepath.IsAbs(file) {
			file = filepath.Join(dataFolder, file)
		}
		db, err = sql.Open("sqlite", file)
		if err == nil {
			db.SetMaxOpenConns(1)
		}
	case "mysql", "mysqli":
		dialect = "mysql"
		m := ToArray(conf.Get("mysql"))
		port := ToInt(m.Get("port"))
		if port == 0 {
			port = 3306
		}
		host := ToString(m.Get("host"))
		if host == "" {
			host = "127.0.0.1"
		}
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=false&multiStatements=true",
			ToString(m.Get("username")), ToString(m.Get("password")), host, port, ToString(m.Get("schema")))
		db, err = sql.Open("mysql", dsn)
	default:
		Throw(NewException("ConfigException", "Unsupported database type \""+dialect+"\". Try \"sqlite\" or \"mysql\"."))
	}
	if err == nil {
		err = db.Ping()
	}
	if err != nil {
		Throw(sqlError("CONNECT", err.Error(), "", nil))
	}
	return &SqlConn{db: db, dialect: dialect, queries: map[string]*sqlQuery{}}
}

// Phar2goSqlDialect is the dialect of a connection ("sqlite" or "mysql").
func Phar2goSqlDialect(conn any) string { return conn.(*SqlConn).dialect }

// Phar2goSqlLogging turns query logging on or off.
func Phar2goSqlLogging(conn any, on bool) { conn.(*SqlConn).logging = on }

// Phar2goSqlLoad parses a libasynql statement file and adds its queries.
func Phar2goSqlLoad(conn any, text string, fileName string) {
	c := conn.(*SqlConn)
	var stack []string
	var buffer, parts []string
	vars := map[string]*sqlVar{}
	parsing := false
	flush := func() {
		parts = append(parts, strings.Join(buffer, "\n"))
		buffer = nil
	}
	for n, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "-- #") {
			if len(stack) == 0 {
				Throw(NewException("GenericStatementFileParseException", fmt.Sprintf("%s:%d: unexpected query text", fileName, n+1)))
			}
			buffer = append(buffer, line)
			parsing = true
			continue
		}
		cmdLine := strings.TrimLeft(line[4:], " \t")
		if cmdLine == "" {
			continue
		}
		args := strings.Fields(cmdLine[1:])
		switch cmdLine[0] {
		case '{':
			if len(args) > 0 {
				stack = append(stack, args[0])
			}
		case '&':
			flush()
		case '}':
			if parsing {
				flush()
				name := strings.Join(stack, ".")
				c.queries[name] = &sqlQuery{name: name, parts: parts, vars: vars}
				parts, vars, parsing = nil, map[string]*sqlVar{}, false
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case ':':
			if len(args) >= 2 {
				v := &sqlVar{name: args[0], typ: args[1]}
				switch {
				case strings.HasPrefix(strings.ToLower(v.typ), "list:"):
					v.list, v.typ = true, v.typ[5:]
				case strings.HasPrefix(strings.ToLower(v.typ), "list?"):
					v.list, v.canEmpty, v.typ = true, true, v.typ[5:]
				case strings.HasPrefix(v.typ, "?"):
					v.nullable, v.typ = true, v.typ[1:]
				}
				if len(args) > 2 {
					def := strings.Join(args[2:], " ")
					v.hasDef = true
					switch v.typ {
					case "string":
						if strings.HasPrefix(def, `"`) && strings.HasSuffix(def, `"`) {
							v.def = ToString(JsonDecode(def))
						} else {
							v.def = def
						}
					case "int":
						v.def = ToInt(def)
					case "float":
						v.def = ToFloat(def)
					case "bool":
						v.def = def == "true" || def == "on" || def == "1"
					default:
						v.def = def
					}
				}
				vars[v.name] = v
			}
			parsing = true
		case '*':
			parsing = true
		}
	}
}

// compile replaces :variables with placeholders and returns the arguments in order.
func (q *sqlQuery) compile(part string, args *Array) (string, []any) {
	names := make([]string, 0, len(q.vars))
	for n := range q.vars {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	var out []any
	var sb strings.Builder
	for i := 0; i < len(part); i++ {
		if part[i] != ':' || i+1 >= len(part) || i > 0 && part[i-1] == ':' {
			sb.WriteByte(part[i])
			continue
		}
		matched := ""
		for _, n := range names {
			end := i + 1 + len(n)
			if strings.HasPrefix(part[i+1:], n) && (end >= len(part) || !isIdentByte(part[end])) {
				matched = n
				break
			}
		}
		if matched == "" {
			sb.WriteByte(part[i])
			continue
		}
		v := q.vars[matched]
		val, ok := args.Lookup(matched)
		if !ok {
			if !v.hasDef && !v.nullable {
				Throw(NewException("InvalidArgumentException", "Missing required variable :"+matched+" in query "+q.name))
			}
			val = v.def
		}
		if v.list {
			vals := ToArray(val).Values()
			if len(vals) == 0 {
				sb.WriteString("(NULL)")
			} else {
				sb.WriteString("(" + strings.TrimSuffix(strings.Repeat("?,", len(vals)), ",") + ")")
				for _, e := range vals {
					out = append(out, sqlValue(v, e))
				}
			}
		} else {
			sb.WriteString("?")
			out = append(out, sqlValue(v, val))
		}
		i += len(matched)
	}
	return sb.String(), out
}

func isIdentByte(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

func sqlValue(v *sqlVar, val any) any {
	if IsNull(val) {
		return nil
	}
	switch v.typ {
	case "int":
		return ToInt(val)
	case "float":
		return ToFloat(val)
	case "bool":
		if ToBool(val) {
			return 1
		}
		return 0
	case "timestamp":
		var t time.Time
		switch strings.ToUpper(ToString(val)) {
		case "NOW":
			t = time.Now()
		case "0":
			t = time.Unix(0, 0)
		default:
			t = time.Unix(int64(ToInt(val)), 0)
		}
		return t.Format("2006-01-02 15:04:05")
	}
	return ToString(val)
}

func sqlError(stage, msg, query string, args *Array) Throwable {
	e := NewException("SqlError", "SQL "+stage+" error: "+msg+func() string {
		if query != "" {
			return ", for query " + query
		}
		return ""
	}())
	e.Code = 0
	return e
}

// Phar2goSqlExecute runs a query (libasynql's executeGeneric/Change/Insert/Select):
// mode 0 returns null, 1 the number of changed rows, 2 [insertId, changedRows], 3 the rows (a
// list of arrays by column name).
func Phar2goSqlExecute(conn any, mode int, name string, args any) any {
	c := conn.(*SqlConn)
	q, ok := c.queries[name]
	if !ok {
		Throw(NewException("InvalidArgumentException", "The query "+name+" has not been loaded"))
	}
	a := ToArray(args)
	var result any
	for i, part := range q.parts {
		text, params := q.compile(part, a)
		last := i == len(q.parts)-1
		if c.logging {
			LogWarning("phar2go: SQL " + text)
		}
		if last && mode == 3 {
			rows, err := c.db.Query(text, params...)
			if err != nil {
				Throw(sqlError("EXECUTION", err.Error(), text, a))
			}
			result = scanRows(rows)
			continue
		}
		res, err := c.db.Exec(text, params...)
		if err != nil {
			Throw(sqlError("EXECUTION", err.Error(), text, a))
		}
		if last {
			affected, _ := res.RowsAffected()
			switch mode {
			case 1:
				result = int(affected)
			case 2:
				id, _ := res.LastInsertId()
				result = List(int(id), int(affected))
			}
		}
	}
	return result
}

func scanRows(rows *sql.Rows) *Array {
	defer rows.Close()
	cols, _ := rows.Columns()
	out := NewArray()
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			Throw(sqlError("RESPONSE", err.Error(), "", nil))
		}
		row := NewArray()
		for i, col := range cols {
			row.Set(col, sqlToPHP(vals[i]))
		}
		out.Append(row)
	}
	return out
}

func sqlToPHP(v any) any {
	switch x := v.(type) {
	case []byte:
		s := string(x)
		if n, err := strconv.Atoi(s); err == nil && strconv.Itoa(n) == s {
			return n
		}
		return s
	case int64:
		return int(x)
	case time.Time:
		return x.Format("2006-01-02 15:04:05")
	}
	return v
}

// Phar2goSqlClose closes a connection.
func Phar2goSqlClose(conn any) {
	if c, ok := conn.(*SqlConn); ok && c.db != nil {
		c.db.Close()
	}
}

// StreamGetContents is stream_get_contents() for the readers Go APIs return (getResource()).
func StreamGetContents(r any, _ ...any) any {
	switch x := r.(type) {
	case io.Reader:
		b, err := io.ReadAll(x)
		if c, ok := x.(io.Closer); ok {
			c.Close()
		}
		if err != nil {
			return false
		}
		return string(b)
	case string:
		return x
	}
	return false
}

// Fclose is fclose() for the readers Go APIs return.
func Fclose(r any) bool {
	if h, ok := r.(*FileHandle); ok {
		if h.f != nil {
			return h.f.Close() == nil
		}
		return true
	}
	if c, ok := r.(io.Closer); ok {
		return c.Close() == nil
	}
	return true
}
