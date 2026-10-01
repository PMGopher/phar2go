<?php

namespace __NS__\base;

use Generator;
use pocketmine\plugin\Plugin;
use __NS__\DataConnector;
use __NS__\SqlError;
use __NS__\SqlThread;

class DataConnectorImpl implements DataConnector{
	private $plugin;
	private $conn;
	private $dialect;
	private $logger = null;
	private $loggingQueries = false;

	public function __construct(Plugin $plugin, $conn, string $dialect){
		$this->plugin = $plugin;
		$this->conn = $conn;
		$this->dialect = $dialect;
	}

	public function setLoggingQueries(bool $loggingQueries) : void{
		$this->loggingQueries = $loggingQueries;
		phar2go_sql_logging($this->conn, $loggingQueries);
	}

	public function isLoggingQueries() : bool{
		return $this->loggingQueries;
	}

	public function setLogger($logger) : void{
		$this->logger = $logger;
	}

	public function getLogger(){
		return $this->logger;
	}

	public function loadQueryFile($fh, ?string $fileName = null) : void{
		phar2go_sql_load($this->conn, stream_get_contents($fh), $fileName ?? "queries.sql");
	}

	/**
	 * Runs a query; the callbacks run right away (on the server thread).
	 */
	private function run(int $mode, string $queryName, array $args, ?callable $onSuccess, ?callable $onError) : void{
		try{
			$result = phar2go_sql_execute($this->conn, $mode, $queryName, $args);
		}catch(\Throwable $e){
			$error = new SqlError(SqlError::STAGE_EXECUTE, $e->getMessage(), $queryName, $args);
			if($onError !== null){
				$onError($error);
				return;
			}
			$this->plugin->getLogger()->error($error->getMessage());
			return;
		}
		if($onSuccess === null){
			return;
		}
		switch($mode){
			case SqlThread::MODE_GENERIC:
				$onSuccess();
				break;
			case SqlThread::MODE_CHANGE:
				$onSuccess($result);
				break;
			case SqlThread::MODE_INSERT:
				$onSuccess($result[0], $result[1]);
				break;
			case SqlThread::MODE_SELECT:
				$columns = [];
				$onSuccess($result, $columns);
				break;
		}
	}

	private function runOrThrow(int $mode, string $queryName, array $args){
		try{
			return phar2go_sql_execute($this->conn, $mode, $queryName, $args);
		}catch(\Throwable $e){
			throw new SqlError(SqlError::STAGE_EXECUTE, $e->getMessage(), $queryName, $args);
		}
	}

	public function executeGeneric(string $queryName, array $args = [], ?callable $onSuccess = null, ?callable $onError = null) : void{
		$this->run(SqlThread::MODE_GENERIC, $queryName, $args, $onSuccess, $onError);
	}

	public function asyncGeneric(string $queryName, array $args = []) : Generator{
		if(false){
			yield;
		}
		$this->runOrThrow(SqlThread::MODE_GENERIC, $queryName, $args);
		return null;
	}

	public function executeChange(string $queryName, array $args = [], ?callable $onSuccess = null, ?callable $onError = null) : void{
		$this->run(SqlThread::MODE_CHANGE, $queryName, $args, $onSuccess, $onError);
	}

	public function asyncChange(string $queryName, array $args = []) : Generator{
		if(false){
			yield;
		}
		return $this->runOrThrow(SqlThread::MODE_CHANGE, $queryName, $args);
	}

	public function executeInsert(string $queryName, array $args = [], ?callable $onInserted = null, ?callable $onError = null) : void{
		$this->run(SqlThread::MODE_INSERT, $queryName, $args, $onInserted, $onError);
	}

	public function asyncInsert(string $queryName, array $args = []) : Generator{
		if(false){
			yield;
		}
		return $this->runOrThrow(SqlThread::MODE_INSERT, $queryName, $args);
	}

	public function executeSelect(string $queryName, array $args = [], ?callable $onSelect = null, ?callable $onError = null) : void{
		$this->run(SqlThread::MODE_SELECT, $queryName, $args, $onSelect, $onError);
	}

	public function asyncSelect(string $queryName, array $args = []) : Generator{
		if(false){
			yield;
		}
		return $this->runOrThrow(SqlThread::MODE_SELECT, $queryName, $args);
	}

	public function asyncSelectWithInfo(string $queryName, array $args = []) : Generator{
		if(false){
			yield;
		}
		return [$this->runOrThrow(SqlThread::MODE_SELECT, $queryName, $args), []];
	}

	public function waitAll() : void{
	}

	public function close() : void{
		phar2go_sql_close($this->conn);
	}
}
