<?php

/**
 * PHP's sqlite3 extension, on Go's database/sql (modernc.org/sqlite).
 */
class SQLite3{
	private mixed $connection;
	private bool $exceptions = false;

	public function __construct(string $filename, int $flags = 6, string $encryptionKey = ""){
		if($filename === ":memory:"){
			$filename = "file::memory:";
		}
		$connection = phar2go_db_open("sqlite", $filename);
		if(is_string($connection)){
			throw new \Exception("Unable to open database: " . $connection);
		}
		$this->connection = $connection;
	}

	public function phar2goConnection() : mixed{
		return $this->connection;
	}

	public function phar2goFailed() : bool{
		if($this->exceptions){
			throw new \Exception($this->lastErrorMsg());
		}
		return false;
	}

	public function enableExceptions(bool $enable = false) : bool{
		$old = $this->exceptions;
		$this->exceptions = $enable;
		return $old;
	}

	public function busyTimeout(int $milliseconds) : bool{
		return true;
	}

	public function exec(string $query) : bool{
		return phar2go_db_exec($this->connection, $query, []) || $this->phar2goFailed();
	}

	public function query(string $query) : SQLite3Result|false{
		if(!phar2go_db_is_query($query)){
			return phar2go_db_exec($this->connection, $query, []) ? new SQLite3Result([], []) : $this->phar2goFailed();
		}
		$result = phar2go_db_query($this->connection, $query, []);
		if($result === false){
			return $this->phar2goFailed();
		}
		return new SQLite3Result($result[0], $result[1]);
	}

	public function querySingle(string $query, bool $entireRow = false) : mixed{
		$result = $this->query($query);
		if($result === false){
			return false;
		}
		$row = $result->fetchArray($entireRow ? SQLITE3_ASSOC : SQLITE3_NUM);
		if($row === false){
			return $entireRow ? [] : null;
		}
		return $entireRow ? $row : $row[0];
	}

	public function prepare(string $query) : SQLite3Stmt|false{
		return new SQLite3Stmt($this, $query);
	}

	public function changes() : int{
		return phar2go_db_changes($this->connection);
	}

	public function lastInsertRowID() : int{
		return phar2go_db_last_insert_id($this->connection);
	}

	public function lastErrorMsg() : string{
		$error = phar2go_db_error($this->connection);
		return $error === "" ? "not an error" : $error;
	}

	public function lastErrorCode() : int{
		return phar2go_db_error_code($this->connection);
	}

	public function close() : bool{
		return phar2go_db_close($this->connection);
	}

	public static function escapeString(string $string) : string{
		return str_replace("'", "''", $string);
	}

	public static function version() : array{
		return ["versionString" => "3", "versionNumber" => 3000000];
	}
}

class SQLite3Stmt{
	private array $params = [];

	public function __construct(private SQLite3 $database, private string $query){
	}

	public function bindValue(string|int $param, mixed $value, int $type = SQLITE3_TEXT) : bool{
		if(is_int($param)){
			$this->params[$param - 1] = $value;
		}else{
			$this->params[$param] = $value;
		}
		return true;
	}

	public function bindParam(string|int $param, mixed $value, int $type = SQLITE3_TEXT) : bool{
		return $this->bindValue($param, $value, $type);
	}

	public function execute() : SQLite3Result|false{
		$connection = $this->database->phar2goConnection();
		ksort($this->params);
		if(!phar2go_db_is_query($this->query)){
			return phar2go_db_exec($connection, $this->query, $this->params) ? new SQLite3Result([], []) : $this->database->phar2goFailed();
		}
		$result = phar2go_db_query($connection, $this->query, $this->params);
		if($result === false){
			return $this->database->phar2goFailed();
		}
		return new SQLite3Result($result[0], $result[1]);
	}

	public function clear() : bool{
		$this->params = [];
		return true;
	}

	public function reset() : bool{
		return true;
	}

	public function close() : bool{
		return true;
	}

	public function paramCount() : int{
		return substr_count($this->query, "?");
	}

	public function getSQL(bool $expand = false) : string{
		return $this->query;
	}
}

class SQLite3Result{
	private int $position = 0;

	public function __construct(private array $columns, private array $rows){
	}

	public function fetchArray(int $mode = SQLITE3_BOTH) : array|false{
		if(!isset($this->rows[$this->position])){
			return false;
		}
		$row = $this->rows[$this->position++];
		$out = [];
		foreach($row as $i => $value){
			if($mode !== SQLITE3_ASSOC){
				$out[$i] = $value;
			}
			if($mode !== SQLITE3_NUM){
				$out[$this->columns[$i]] = $value;
			}
		}
		return $out;
	}

	public function numColumns() : int{
		return count($this->columns);
	}

	public function columnName(int $column) : string|false{
		return $this->columns[$column] ?? false;
	}

	public function columnType(int $column) : int|false{
		return isset($this->columns[$column]) ? SQLITE3_TEXT : false;
	}

	public function reset() : bool{
		$this->position = 0;
		return true;
	}

	public function finalize() : bool{
		return true;
	}
}
