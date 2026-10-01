<?php

// phar2go's replacement for libSQL: the queries run on Go's database/sql (phar2go's SQLite3 and
// mysqli classes) on the server's thread, and their callbacks are called on the next tick.

namespace __NS__;

use Closure;
use __NS__\exception\SQLException;
use __NS__\query\SQLQuery;
use pocketmine\plugin\PluginBase;
use pocketmine\scheduler\ClosureTask;
use pocketmine\utils\SingletonTrait;

final class ConnectionPool
{
    use SingletonTrait {
        reset as protected;
        setInstance as protected;
    }

    private mixed $connection;

    public function __construct(protected PluginBase $plugin, array $configuration)
    {
        self::setInstance($this);
        if ($configuration["provider"] === "mysql") {
            $mysql = $configuration["mysql"];
            $this->connection = new \mysqli($mysql["host"] ?? "127.0.0.1", $mysql["username"] ?? "", $mysql["password"] ?? "", $mysql["database"] ?? ($mysql["schema"] ?? ""), (int) ($mysql["port"] ?? 3306));
            if ($this->connection->connect_error !== null) {
                throw new \RuntimeException($this->connection->connect_error);
            }
        } else {
            $this->connection = new \SQLite3($plugin->getDataFolder() . $configuration["sqlite"]["path"]);
        }
    }

    public function submit(SQLQuery $query, ?Closure $onSuccess = null, ?Closure $onFail = null): void
    {
        $error = null;
        try {
            $query->onRun($this->connection);
        } catch (\Throwable $throwable) {
            $error = $throwable instanceof SQLException ? $throwable : new SQLException(_message: $throwable->getMessage(), _code: (int) $throwable->getCode());
        }
        $this->plugin->getScheduler()->scheduleTask(new ClosureTask(function () use ($query, $error, $onSuccess, $onFail): void {
            if ($error === null) {
                if ($onSuccess !== null) {
                    $onSuccess($query->getResult());
                }
            } elseif ($onFail !== null) {
                $onFail($error);
            } else {
                $this->plugin->getLogger()->logException($error);
            }
        }));
    }
}
