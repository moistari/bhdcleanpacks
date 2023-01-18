# about

does some stuff.

## using

```sh
# checkout code
$ git clone https://github.com/moistari/bhdcleanpacks.git && cd bhdcleanpacks

# build and run. replace <APIKEY> and <RSSKEY> with your bhd API and RSS keys,
# respectively. script will output some debugging info:
$ go build && ./bhdcleanpacks -api <APIKEY> -rss <RSSKEY>
...
1701fd3: "The Right Stuff S01 2160p DSNP WEB-DL DDP 5.1 Atmos DV HDR H 265-CRFW" 238812
1701fd3: "The Right Stuff S01 2160p DSNP WEB-DL DDP 5.1 Atmos DV HDR H 265-CRFW" 238812 remove: 1
...

# alternately, set APIKEY and RSSKEY in environment, and build/run
$ export APIKEY=<APIKEY> RSSKEY=<RSSKEY>
$ go build && ./bhdcleanpacks
...
1701fd3: "The Right Stuff S01 2160p DSNP WEB-DL DDP 5.1 Atmos DV HDR H 265-CRFW" 238812
1701fd3: "The Right Stuff S01 2160p DSNP WEB-DL DDP 5.1 Atmos DV HDR H 265-CRFW" 238812 remove: 1
...

# actual packs to clean up will be in YYYY-MM-DD.yml:
$ cat 2024-01-18.yml
---
name: "The Right Stuff S01 2160p DSNP WEB-DL DDP 5.1 Atmos DV HDR H 265-CRFW"
hash: 1701fd3
id: 238812
url: https://beyond-hd.me/torrents/the-right-stuff-s01-2160p-dsnp-web-dl-ddp-51-atmos-dv-hdr-h-265-crfw.238812
replaces:
-  name: "The Right Stuff S01E01 Sierra Hotel 2160p WEB-DL DDP 5.1 Atmos x265-MZABI"
   hash: 9a121ba
   id: 87122
   url: https://beyond-hd.me/torrents/the-right-stuff-s01e01-sierra-hotel-2160p-web-dl-ddp-51-atmos-x265-mzabi.87122
...

# there are other useful command line parameters:
./bhdcleanpacks --help
Usage of ./bhdcleanpacks:
  -cache-dir string
    	cache directory
  -d duration
    	duration (default 48h0m0s)
  -debug
    	debug
  -key string
    	bhd api key
  -out string
    	out (default "2006-01-02.yml")
  -rss string
    	bhd rss key
```
