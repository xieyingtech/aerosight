#!/bin/sh
set -eu

validate_value() {
  name="$1"
  eval "value=\${$name}"
  case "$value" in
    ""|*[!A-Za-z0-9._-]*)
      echo "$name must contain only letters, digits, dot, underscore, or hyphen" >&2
      exit 1
      ;;
  esac
}

for name in MQTT_DEVICE_USER MQTT_DEVICE_PASSWORD MQTT_PLATFORM_USER MQTT_PLATFORM_PASSWORD \
  MEDIA_PUBLISH_USER MEDIA_PUBLISH_PASSWORD MEDIA_READ_USER MEDIA_READ_PASSWORD MEDIA_ADMIN_USER MEDIA_ADMIN_PASSWORD; do
  validate_value "$name"
done

umask 077
rm -f /generated/mosquitto.passwords
mosquitto_passwd -b -c /generated/mosquitto.passwords "$MQTT_DEVICE_USER" "$MQTT_DEVICE_PASSWORD"
mosquitto_passwd -b /generated/mosquitto.passwords "$MQTT_PLATFORM_USER" "$MQTT_PLATFORM_PASSWORD"

sed \
  -e "s/__MQTT_DEVICE_USER__/$MQTT_DEVICE_USER/g" \
  -e "s/__MQTT_PLATFORM_USER__/$MQTT_PLATFORM_USER/g" \
  /templates/mosquitto.acl.template > /generated/mosquitto.acl

sed \
  -e "s/__MEDIA_PUBLISH_USER__/$MEDIA_PUBLISH_USER/g" \
  -e "s/__MEDIA_PUBLISH_PASSWORD__/$MEDIA_PUBLISH_PASSWORD/g" \
  -e "s/__MEDIA_READ_USER__/$MEDIA_READ_USER/g" \
  -e "s/__MEDIA_READ_PASSWORD__/$MEDIA_READ_PASSWORD/g" \
  -e "s/__MEDIA_ADMIN_USER__/$MEDIA_ADMIN_USER/g" \
  -e "s/__MEDIA_ADMIN_PASSWORD__/$MEDIA_ADMIN_PASSWORD/g" \
  /templates/mediamtx.yml.template > /generated/mediamtx.yml
chown mosquitto:mosquitto /generated/mosquitto.passwords /generated/mosquitto.acl
chmod 600 /generated/mosquitto.passwords
chmod 640 /generated/mosquitto.acl
chmod 600 /generated/mediamtx.yml
