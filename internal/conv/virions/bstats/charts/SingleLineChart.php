<?php

namespace __NS__\charts;

class SingleLineChart extends CustomChart{
	public function __construct(string $chartId, $callable = null){
		parent::__construct($chartId);
	}
}
