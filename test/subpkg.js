GopherJS.extend("github.com/gopherjs/gopherjs/test/subpkg", async function() {
    var $pkg = {}, $init, init, init$1, Data;
    init = function init$2() {
        $pkg.DataStr = "Hello World";
    };
    init$1 = function init$3() {
        $pkg.DataStr = "Hello World";
    };
    Data = function Data$1() {
        return $pkg.DataStr;
    };
    $pkg.Data = Data;
    $init = function() {
        $pkg.$init = function() {};
        /* */ var $f, $c = false, $s = 0, $r; if (this !== undefined && this.$blk !== undefined) { $f = this; $c = true; $s = $f.$s; $r = $f.$r; } s: while (true) { switch ($s) { case 0:
            $pkg.DataStr = "";
            init();
            init$1();
            /* */ } return; } if ($f === undefined) { $f = { $blk: $init }; } $f.$s = $s; $f.$r = $r; return $f;
    };
    $pkg.$init = $init;
    return $pkg;
});
