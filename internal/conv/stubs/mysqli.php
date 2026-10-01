<?php

/**
 * PHP's mysqli extension, on Go's database/sql (github.com/go-sql-driver/mysql).
 */
class mysqli{
	public int|string $affected_rows = 0;
	public int|string $insert_id = 0;
	public string $error = "";
	public int $errno = 0;
	public ?string $connect_error = null;
	public int $connect_errno = 0;
	private mixed $connection = null;

	public function __construct(?string $hostname = null, ?string $username = null, ?string $password = null, ?string $database = null, ?int $port = null, ?string $socket = null){
		if($hostname !== null){
			$this->real_connect($hostname, $username, $password, $database, $port);
		}
	}

	public function real_connect(?string $hostname = null, ?string $username = null, ?string $password = null, ?string $database = null, ?int $port = null, ?string $socket = null, int $flags = 0) : bool{
		$connection = phar2go_db_open("mysql", phar2go_db_mysql_dsn($hostname ?? "127.0.0.1", $username ?? "", $password ?? "", $database ?? "", $port ?? 3306));
		if(is_string($connection)){
			$this->connect_error = $connection;
			$this->connect_errno = 2002;
			return false;
		}
		$this->connection = $connection;
		return true;
	}

	public function phar2goConnection() : mixed{
		return $this->connection;
	}

	public function phar2goUpdate() : void{
		$this->affected_rows = phar2go_db_changes($this->connection);
		$this->insert_id = phar2go_db_last_insert_id($this->connection);
		$this->error = phar2go_db_error($this->connection);
		$this->errno = phar2go_db_error_code($this->connection);
	}

	public function query(string $query, int $resultMode = MYSQLI_STORE_RESULT) : mysqli_result|bool{
		if(!phar2go_db_is_query($query)){
			$ok = phar2go_db_exec($this->connection, $query, []);
			$this->phar2goUpdate();
			return $ok;
		}
		$result = phar2go_db_query($this->connection, $query, []);
		$this->phar2goUpdate();
		return $result === false ? false : new mysqli_result($result[0], $result[1]);
	}

	public function multi_query(string $query) : bool{
		return $this->query($query) !== false;
	}

	public function prepare(string $query) : mysqli_stmt|false{
		return new mysqli_stmt($this, $query);
	}

	public function begin_transaction(int $flags = 0, ?string $name = null) : bool{
		return phar2go_db_begin($this->connection);
	}

	public function autocommit(bool $enable) : bool{
		return $enable ? true : phar2go_db_begin($this->connection);
	}

	public function commit(int $flags = 0, ?string $name = null) : bool{
		return phar2go_db_commit($this->connection);
	}

	public function rollback(int $flags = 0, ?string $name = null) : bool{
		return phar2go_db_rollback($this->connection);
	}

	public function ping() : bool{
		return $this->connection !== null;
	}

	public function select_db(string $database) : bool{
		return $this->query("USE `" . str_replace("`", "``", $database) . "`") !== false;
	}

	public function set_charset(string $charset) : bool{
		return true;
	}

	public function real_escape_string(string $string) : string{
		return addslashes($string);
	}

	public function escape_string(string $string) : string{
		return addslashes($string);
	}

	public function close() : bool{
		return $this->connection !== null && phar2go_db_close($this->connection);
	}
}

class mysqli_stmt{
	public int|string $affected_rows = 0;
	public int|string $insert_id = 0;
	public int|string $num_rows = 0;
	public string $error = "";
	public int $errno = 0;
	private array $params = [];
	private mysqli_result|false $result = false;

	public function __construct(private mysqli $mysql, private string $query){
	}

	public function bind_param(string $types, mixed ...$vars) : bool{
		$this->params = [];
		foreach($vars as $i => $value){
			$this->params[] = match($types[$i] ?? "s"){
				"i" => (int) $value,
				"d" => (float) $value,
				default => $value,
			};
		}
		return true;
	}

	public function execute(?array $params = null) : bool{
		if($params !== null){
			$this->params = array_values($params);
		}
		$connection = $this->mysql->phar2goConnection();
		if(!phar2go_db_is_query($this->query)){
			$ok = phar2go_db_exec($connection, $this->query, $this->params);
			$this->result = false;
		}else{
			$rows = phar2go_db_query($connection, $this->query, $this->params);
			$ok = $rows !== false;
			$this->result = $ok ? new mysqli_result($rows[0], $rows[1]) : false;
			$this->num_rows = $ok ? count($rows[1]) : 0;
		}
		$this->mysql->phar2goUpdate();
		$this->affected_rows = $this->mysql->affected_rows;
		$this->insert_id = $this->mysql->insert_id;
		$this->error = $this->mysql->error;
		$this->errno = $this->mysql->errno;
		return $ok;
	}

	public function get_result() : mysqli_result|false{
		return $this->result;
	}

	public function store_result() : bool{
		return true;
	}

	public function free_result() : void{
	}

	public function reset() : bool{
		return true;
	}

	public function close() : bool{
		return true;
	}
}

class mysqli_result{
	public int $num_rows = 0;
	public int $field_count = 0;
	private int $position = 0;

	public function __construct(private array $columns, private array $rows){
		$this->num_rows = count($rows);
		$this->field_count = count($columns);
	}

	public function fetch_array(int $mode = MYSQLI_BOTH) : array|null|false{
		if(!isset($this->rows[$this->position])){
			return null;
		}
		$row = $this->rows[$this->position++];
		$out = [];
		foreach($row as $i => $value){
			if($mode !== MYSQLI_ASSOC){
				$out[$i] = $value;
			}
			if($mode !== MYSQLI_NUM){
				$out[$this->columns[$i]] = $value;
			}
		}
		return $out;
	}

	public function fetch_assoc() : array|null|false{
		return $this->fetch_array(MYSQLI_ASSOC);
	}

	public function fetch_row() : array|null|false{
		return $this->fetch_array(MYSQLI_NUM);
	}

	public function fetch_all(int $mode = MYSQLI_NUM) : array{
		$out = [];
		while(($row = $this->fetch_array($mode)) !== null){
			$out[] = $row;
		}
		return $out;
	}

	public function data_seek(int $offset) : bool{
		$this->position = $offset;
		return isset($this->rows[$offset]);
	}

	public function free() : void{
	}

	public function close() : void{
	}

	public function free_result() : void{
	}
}
