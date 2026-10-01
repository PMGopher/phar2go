<?php

namespace __NS__\query;

use SQLite3;

abstract class SQLiteQuery extends SQLQuery
{
    abstract public function onRun(SQLite3 $connection): void;
}
