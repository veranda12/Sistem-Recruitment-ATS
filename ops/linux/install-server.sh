#!/usr/bin/env bash
# Persiapan server sekali jalan (root): user terbatas, direktori, file environment terlindungi, unit systemd.
set -euo pipefail

id ats >/dev/null 2>&1 || useradd --system --home /var/lib/ats --shell /usr/sbin/nologin ats
install -d -o ats -g ats -m 0750 /var/lib/ats /var/lib/ats/objects /var/lib/ats/mail
install -d -o root -g ats -m 0750 /etc/ats /opt/ats /opt/ats/releases
install -d -o root -g root -m 0755 /var/www/ats

if [ ! -f /etc/ats/ats.env ]; then
  install -o root -g ats -m 0640 /dev/null /etc/ats/ats.env
  echo "Isi /etc/ats/ats.env dari ats.env.example. Buat kunci: head -c 32 /dev/urandom | base64"
fi

cp ops/systemd/ats-api@.service ops/systemd/ats-worker.service ops/systemd/ats-migrate.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable ats-api@8080 ats-api@8082 ats-worker
cp ops/nginx/ats.conf /etc/nginx/conf.d/ats.conf
nginx -t
echo "Server siap. Deploy rilis: ops/linux/deploy.sh dist/ats-<versi>-linux-amd64.tar.gz"
