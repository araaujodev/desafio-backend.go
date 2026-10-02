#!/bin/bash
go build -o /tmp/api ./cmd/api || exit 1
PIDS=""
for p in 8080 8082 8083; do HTTP_ADDR=":$p" /tmp/api >/tmp/api$p.log 2>&1 & PIDS="$PIDS $!"; done
sleep 6
K=http://localhost:8081/realms/wager/protocol/openid-connect/token
tok() { curl -s -X POST $K -d grant_type=client_credentials -d client_id=$1 -d client_secret=$2 | python3 -c "import sys,json;print(json.load(sys.stdin)['access_token'])"; }
INT=$(tok internal-service internal-local-secret); PA=$(tok provider-a provider-a-local-secret)
PORTS=(8080 8082 8083)
newwallet() { P=$(python3 -c "import uuid;print(uuid.uuid4())"); curl -s -X POST localhost:8080/wallets -H "Authorization: Bearer $INT" -d "{\"playerId\":\"$P\",\"initialBalance\":{\"amount\":\"100.00\",\"currency\":\"BRL\"}}" | python3 -c "import sys,json;d=json.load(sys.stdin);print(d['id'],d['playerId'])"; }
body() { echo "{\"providerId\":\"provider-a\",\"externalTransactionId\":\"$1\",\"playerId\":\"$2\",\"walletId\":\"$3\",\"roundId\":\"r1\",\"gameId\":\"g1\",\"kind\":\"BET\",\"money\":{\"amount\":\"$4\",\"currency\":\"BRL\"}}"; }
send() { curl -s -X POST localhost:$1/wagering/transactions -H "Authorization: Bearer $PA" -H "Idempotency-Key: k-$2" -d "$3"; echo; }

echo "== TESTE 1: mesma aposta 30x em 3 processos"
read W P <<< "$(newwallet)"
B=$(body same-$RANDOM $P $W 25.00)
EXT=$(echo $B | python3 -c "import sys,json;print(json.load(sys.stdin)['externalTransactionId'])")
( for i in $(seq 1 30); do send ${PORTS[$((i%3))]} $EXT "$B" > /tmp/r1_$i.txt & done; wait )
echo "originais (esperado 1): $(cat /tmp/r1_*.txt | grep -c '"idempotentReplay":false')"
echo "replays   (esperado 29): $(cat /tmp/r1_*.txt | grep -c '"idempotentReplay":true')"
curl -s -X POST localhost:8082/wallets/$W/reconciliation -H "Authorization: Bearer $INT"; echo

echo "== TESTE 2: duas apostas de 80.00 sobre 100.00 em processos diferentes"
read W P <<< "$(newwallet)"
( send 8080 a$RANDOM "$(body x1-$RANDOM $P $W 80.00)" > /tmp/r2_1.txt & send 8083 b$RANDOM "$(body x2-$RANDOM $P $W 80.00)" > /tmp/r2_2.txt & wait )
cat /tmp/r2_1.txt /tmp/r2_2.txt
curl -s -X POST localhost:8082/wallets/$W/reconciliation -H "Authorization: Bearer $INT"; echo

kill $PIDS
