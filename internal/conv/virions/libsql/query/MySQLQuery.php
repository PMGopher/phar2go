<?php

namespace __NS__\query;

use mysqli;

abstract class MySQLQuery extends SQLQuery
{
    abstract public function onRun(mysqli $connection): void;
}
