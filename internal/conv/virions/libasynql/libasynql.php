<?php

// phar2go's replacement for libasynql: the same API, running the queries with Go's
// database/sql (see phpx/sql.go) instead of PHP threads and extensions.

namespace __NS__;

use pocketmine\plugin\PluginBase;
use __NS__\base\DataConnectorImpl;

final class libasynql{
	private static $packaged = true;

	public static function isPackaged() : bool{
		return self::$packaged;
	}

	public static function detectPackaged() : void{
	}

	public static function create(PluginBase $plugin, $configData, array $sqlMap, ?bool $logQueries = null) : DataConnector{
		if(!is_array($configData)){
			throw new ConfigException("Missing database settings");
		}
		$conn = phar2go_sql_open($configData, $plugin->getDataFolder());
		$dialect = phar2go_sql_dialect($conn);
		if(!isset($sqlMap[$dialect])){
			throw new ConfigException("Database type $dialect is not supported by this plugin");
		}
		$connector = new DataConnectorImpl($plugin, $conn, $dialect);
		$connector->setLoggingQueries($logQueries ?? false);
		foreach((is_array($sqlMap[$dialect]) ? $sqlMap[$dialect] : [$sqlMap[$dialect]]) as $file){
			$resource = $plugin->getResource($file);
			if($resource === null){
				throw new ConfigException("Resource $file not found");
			}
			$connector->loadQueryFile($resource, $file);
		}
		return $connector;
	}
}
