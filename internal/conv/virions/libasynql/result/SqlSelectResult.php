<?php

namespace __NS__\result;

class SqlSelectResult{
	private $rows;

	public function __construct(array $rows){
		$this->rows = $rows;
	}

	public function getRows() : array{
		return $this->rows;
	}
}
