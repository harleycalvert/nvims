echo "==> Restarting Go server..."
fuser -k 8080/tcp 2>/dev/null || true
sleep 1
cd "$HOME/nvims-sms/src"
nohup go run ./cmd/server >> /tmp/nvims-server.log 2>&1 &
echo "==> Server restarted (PID $!, log: /tmp/nvims-server.log)"

echo "==> Done."