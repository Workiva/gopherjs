var $packages = {};

var $mainPkg;
var $mainPkgName;
var $cacheLookupUrl;

var $initAllLinknames = () => {
    var names = $keys($packages);
    for (var i = 0; i < names.length; i++) {
        var f = $packages[names[i]]["$initLinknames"];
        if (typeof f == 'function') {
            f();
        }
    }
}

var $getPackage = async (packageName) => {
    if ($packages[packageName] === undefined) {
        await loadScript($cacheLookupUrl + packageName + ".js")
    }
    return $packages[packageName];
}

var loadScript = (src) => {
    return new Promise((resolve, reject) => {
        const script = document.createElement('script');
        script.src = src;
        script.async = false;

        script.onload = () => {
            resolve();
        };

        script.onerror = () => {
            reject(new Error(`Failed to load script: ${src}`));
        };

        document.head.appendChild(script);
    });
}

$global.GopherJS = {}
$global.GopherJS.extend = async (packageName, pkgFn) => {
    $packages[packageName] = await pkgFn();
}

$global.GopherJS.startup = async () => {
    var $mainPkg = await $getPackage($mainPkgName);

    $synthesizeMethods();
    $initAllLinknames();

    (await $getPackage("runtime")).$init();
    $go($mainPkg.$init, []);

    $flushConsole();
}
