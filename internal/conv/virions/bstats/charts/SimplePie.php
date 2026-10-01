<?php

namespace __NS__\charts;

class SimplePie extends CustomChart{
	public function __construct(string $chartId, $callable = null){
		parent::__construct($chartId);
	}
}
