<?php

$name = ucfirst($api['interface']);
$filename = $dirname . "/router.go";

$tpl->load("http_routes.tpl");
$tpl->assign($common);
$tpl->assign("package", basename($dirname));
$tpl->assign("name", $name);
$tpl->assign("api", $api);
$tpl->assign("apis", $apis);
$tpl->assign("self", strtolower(substr($name, 0, 1)));
$tpl->assign("structs", $api['struct']);
$imports = array();
foreach ($api['struct'] as $struct) {
	if (isset($struct['imports']))
	foreach ($struct['imports'] as $import) {
		$imports[] = $import;
	}
}
$tpl->assign("imports", $imports);
$tpl->assign("calls", $api['apis']);
$contents = str_replace("\n\n}", "\n}", $tpl->get());

file_put_contents($filename, $contents);
echo $filename . "\n";

foreach ($apis as $api) {
	if (is_array($api['struct'])) {
		$name = ucfirst($api['interface']);
		$filename = $dirname . "/" . str_replace("..", ".", strtolower($name) . ".go");

		$tpl->load("http_.tpl");
		$tpl->assign($common);
		$tpl->assign("package", basename(__DIR__));
		$tpl->assign("name", $name);
		$tpl->assign("api", $api);
		$tpl->assign("self", strtolower(substr($name, 0, 1)));
		$tpl->assign("structs", $api['struct']);
		$imports = array();
		foreach ($api['struct'] as $struct) {
			if (isset($struct['imports']))
			foreach ($struct['imports'] as $import) {
				$imports[] = $import;
			}
		}
		$tpl->assign("imports", $imports);
		$tpl->assign("calls", $api['apis']);
		$contents = str_replace("\n\n}", "\n}", $tpl->get());

		if (!file_exists($filename)) {
			file_put_contents($filename, $contents);
			echo $filename . "\n";
		}
	}
}
