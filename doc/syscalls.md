## System Calls

System calls are the bridge between your application and your operating system. They are used whenever you access something outside of your application's memory, for example when you write to the console, when you read or write files or when you access the network. In Go, system calls are mostly used by the `os` package, hence the name. When using GopherJS you need to consider if system calls are available or not.

Starting with 1.18, GopherJS provides the same [set of cross-platform](https://pkg.go.dev/syscall?GOOS=js) syscalls as standard Go WebAssembly, emulating them via JavaScript APIs available in the runtime (browser or Node.js).

### Output redirection to console

If system calls are not available in your environment (see below), then a special redirection of `os.Stdout` and `os.Stderr` is applied. It buffers a line until it is terminated by a line break and then prints it via JavaScript's `console.log` to your browser's JavaScript console or your system console. That way, `fmt.Println` etc. work as expected, even if system calls are not available.

### In Browser

The JavaScript environment of a web browser is completely isolated from your operating system to protect your machine. You don't want any web page to read or write files on your disk without your consent. That is why system calls are not and will never be available when running your code in a web browser.

However, certain subsets of syscalls can be emulated using third-party libraries. For example, [BrowserFS](https://github.com/jvilk/BrowserFS) library can be used to emulate Node.js file system API in a browser using HTML5 LocalStorage or other fallbacks.

### Node.js on all platforms

GopherJS emulates syscalls for accessing file system (and a few others) using Node.js standard [`fs`](https://nodejs.org/api/fs.html) and [`process`](https://nodejs.org/api/process.html) APIs. No additional extensions are required for this in GopherJS 1.18 and newer.
