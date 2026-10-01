<?php

namespace __NS__;

use Generator;

interface DataConnector{
	public function setLoggingQueries(bool $loggingQueries) : void;

	public function isLoggingQueries() : bool;

	public function setLogger($logger) : void;

	public function getLogger();

	public function loadQueryFile($fh, ?string $fileName = null) : void;

	public function executeGeneric(string $queryName, array $args = [], ?callable $onSuccess = null, ?callable $onError = null) : void;

	public function asyncGeneric(string $queryName, array $args = []) : Generator;

	public function executeChange(string $queryName, array $args = [], ?callable $onSuccess = null, ?callable $onError = null) : void;

	public function asyncChange(string $queryName, array $args = []) : Generator;

	public function executeInsert(string $queryName, array $args = [], ?callable $onInserted = null, ?callable $onError = null) : void;

	public function asyncInsert(string $queryName, array $args = []) : Generator;

	public function executeSelect(string $queryName, array $args = [], ?callable $onSelect = null, ?callable $onError = null) : void;

	public function asyncSelect(string $queryName, array $args = []) : Generator;

	public function waitAll() : void;

	public function close() : void;
}
