# Third-party notices

## Cloudflare API schemas

`internal/cloudflare/catalog.json.gz` is generated from the OpenAPI schema published at
[cloudflare/api-schemas](https://github.com/cloudflare/api-schemas): operation names, paths,
parameters, request schemas and descriptions come from it. That schema is distributed under the
following license.

```
BSD 3-Clause License

Copyright (c) 2022, Cloudflare
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its
   contributors may be used to endorse or promote products derived from
   this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

## Generated Portuguese texts

`internal/cloudflare/descriptions_pt.json` and `internal/cloudflare/queries_pt.json` were written by
a language model (Claude Haiku) from the schema above, with `scripts/gen_pt_texts.py`. They are part
of this project and covered by its MIT license.

## Trademarks

Cloudflare is a trademark of Cloudflare, Inc. This project is not affiliated with, endorsed by or
sponsored by Cloudflare, Inc. The name only says which API the server talks to.
