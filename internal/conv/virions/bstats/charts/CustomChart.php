<?php

namespace __NS__\charts;

abstract class CustomChart{
	private string $chartId;

	public function __construct(string $chartId){
		$this->chartId = $chartId;
	}

	public function getChartId() : string{
		return $this->chartId;
	}
}
