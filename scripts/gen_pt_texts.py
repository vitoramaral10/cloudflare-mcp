#!/usr/bin/env python3
"""Generates, for each catalog operation, the Portuguese text the find search indexes.

    --kind descriptions  one sentence about the task  -> internal/cloudflare/descriptions_pt.json {name: sentence}
    --kind queries       5 user requests              -> internal/cloudflare/queries_pt.json {name: [requests]}

Uses the `claude` CLI (Haiku) in batches. Resumable: only asks for the operations still missing.
Run again after regenerating the catalog; removed operations are dropped.

    python3 scripts/gen_pt_texts.py --kind queries [--batch 40] [--parallel 4] [--limit N]
"""
import argparse
import concurrent.futures as cf
import gzip
import json
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
CATALOG = ROOT / "internal/cloudflare/catalog.json.gz"
OUTPUTS = {"descriptions": ROOT / "internal/cloudflare/descriptions_pt.json",
           "queries": ROOT / "internal/cloudflare/queries_pt.json"}

PROMPT = """Você recebe operações da API da Cloudflare (nome, método, caminho, tags, descrição em inglês).
Para cada uma, escreva UMA frase em português do Brasil, de 8 a 25 palavras, dizendo para que
um usuário comum usaria a operação: a tarefa, com as palavras que ele usaria ao pedir ajuda
(ex.: "domínio", "site", "apontar", "liberar", "bloquear", "apagar", "ver"), seguida, se couber,
do nome do recurso da Cloudflare. Não traduza ao pé da letra; não invente recursos.
Responda SOMENTE com um objeto JSON {"nome_da_operacao": "frase", ...}, sem texto fora dele.

Operações:
"""

PROMPT_QUERIES = """Você recebe operações da API da Cloudflare (nome, método, caminho, tags, descrição).
Para cada uma, escreva 5 pedidos diferentes que um usuário brasileiro digitaria para um assistente
quando precisa exatamente dessa operação. Varie: 2 informais (como no WhatsApp, com gíria leve e sem
termo técnico, descrevendo o problema ou o objetivo), 2 diretos (verbo + recurso, com o nome que o
usuário daria: domínio, site, registro, túnel, regra, bucket, worker...) e 1 com o termo técnico da
Cloudflare. Cada pedido com 4 a 15 palavras. Não invente o que a operação não faz.
Responda SOMENTE com um objeto JSON {"nome_da_operacao": ["pedido 1", ..., "pedido 5"], ...}, sem texto fora dele.

Operações:
"""


def load_ops():
    with gzip.open(CATALOG, "rt") as f:
        return json.load(f)


def summary(op):
    return {"nome": op["n"], "metodo": op["m"], "caminho": op["p"],
            "tags": op.get("t", []), "descricao": op.get("d", "")[:300]}


def ask(batch, kind):
    prompt = PROMPT if kind == "descriptions" else PROMPT_QUERIES
    payload = prompt + json.dumps([summary(o) for o in batch], ensure_ascii=False)
    r = subprocess.run(["claude", "-p", "--model", "haiku", "--output-format", "text"],
                       input=payload, capture_output=True, text=True, timeout=600)
    if r.returncode != 0:
        raise RuntimeError(r.stderr.strip()[:300])
    text = r.stdout.strip()
    start, end = text.find("{"), text.rfind("}")
    data = json.loads(text[start:end + 1])
    names = {o["n"] for o in batch}
    if kind == "descriptions":
        return {k: v.strip() for k, v in data.items() if k in names and isinstance(v, str) and v.strip()}
    return {k: [x.strip() for x in v if isinstance(x, str) and x.strip()][:6]
            for k, v in data.items() if k in names and isinstance(v, list) and len(v) >= 3}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--kind", choices=sorted(OUTPUTS), default="descriptions")
    ap.add_argument("--batch", type=int, default=80)
    ap.add_argument("--parallel", type=int, default=4)
    ap.add_argument("--limit", type=int, default=0, help="only the first N pending (testing)")
    a = ap.parse_args()

    out_path = OUTPUTS[a.kind]
    ops = load_ops()
    done = json.loads(out_path.read_text()) if out_path.exists() else {}
    names = {o["n"] for o in ops}
    done = {k: v for k, v in done.items() if k in names}
    pending = [o for o in ops if o["n"] not in done]
    if a.limit:
        pending = pending[:a.limit]
    batches = [pending[i:i + a.batch] for i in range(0, len(pending), a.batch)]
    print(f"{len(done)} done, {len(pending)} pending in {len(batches)} batches", file=sys.stderr)

    def save():
        tmp = out_path.with_suffix(".tmp")
        tmp.write_text(json.dumps(dict(sorted(done.items())), ensure_ascii=False, indent=0) + "\n")
        tmp.replace(out_path)

    with cf.ThreadPoolExecutor(a.parallel) as ex:
        futures = {ex.submit(ask, b, a.kind): i for i, b in enumerate(batches)}
        for fut in cf.as_completed(futures):
            i = futures[fut]
            try:
                new = fut.result()
                done.update(new)
                save()
                print(f"batch {i + 1}/{len(batches)}: {len(new)}/{len(batches[i])}", file=sys.stderr)
            except Exception as e:  # a failed batch stays pending for the next run
                print(f"batch {i + 1}/{len(batches)} failed: {e}", file=sys.stderr)
    missing = len(names) - len(done)
    print(f"total: {len(done)} done, {missing} missing", file=sys.stderr)
    sys.exit(1 if missing else 0)


if __name__ == "__main__":
    main()
