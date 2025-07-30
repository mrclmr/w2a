# Design from v0.6.0

First all output filenames with their hashes are calculated. All commands and their output filenames are then stored in a directed acyclic graph. All commands are represented as nodes in the graph. Each non-existent final playable audio file - represents a root node in this graph - is executed in parallel. This makes creating audio files much faster:

```
MacBook Pro 2023, M2 Pro, 32GB Ram
    tts command: say
convert command: afconvert

w2a version 0.3.0
real	5m33.128s
user	0m46.988s
sys	0m15.027s

w2a version 0.6.0
real	0m20.943s
user	0m16.639s
sys	0m8.005s
```