<?php

namespace __NS__;

use RuntimeException;

class SqlError extends RuntimeException{
	public const STAGE_CONNECT = "CONNECT";
	public const STAGE_PREPARE = "PREPARE";
	public const STAGE_EXECUTE = "EXECUTION";
	public const STAGE_RESPONSE = "RESPONSE";

	private $stage;
	private $errorMessage;
	private $query;
	private $args;

	public function __construct(string $stage, string $errorMessage, ?string $query = null, ?array $args = null){
		$this->stage = $stage;
		$this->errorMessage = $errorMessage;
		$this->query = $query;
		$this->args = $args;
		parent::__construct("SQL $stage error: $errorMessage" . ($query === null ? "" : ", for query $query"));
	}

	public function getStage() : string{
		return $this->stage;
	}

	public function getErrorMessage() : string{
		return $this->errorMessage;
	}

	public function getQuery() : ?string{
		return $this->query;
	}

	public function getArgs() : ?array{
		return $this->args;
	}
}
