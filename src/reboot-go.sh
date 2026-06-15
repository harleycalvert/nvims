echo "==> Restarting PostgreSQL..."
sudo systemctl restart postgresql@16-main
echo "==> PostgreSQL restarted."

echo "==> Restarting MinIO..."
sudo systemctl restart minio
echo "==> MinIO restarted."

echo "==> Building Go server..."
cd "$HOME/nvims-sms/src"
go build -o "$HOME/nvims-sms/nvims-sms" ./cmd/server
echo "==> Restarting Go server..."
sudo systemctl restart nvims

echo "==> Server restarted."
echo "==> Done."