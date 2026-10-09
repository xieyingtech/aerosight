-- Presentation belongs to DeviceType, shared by every device and map view.
ALTER TABLE device_types ADD COLUMN icon text NOT NULL DEFAULT 'cpu'
  CHECK (icon ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$' AND length(icon) <= 80);

UPDATE device_types SET icon = 'warehouse' WHERE type_key IN ('dji.dock2', 'dji.dock3');
UPDATE device_types SET icon = 'drone' WHERE type_key IN ('dji.matrice3d', 'dji.matrice3td', 'dji.matrice4d', 'dji.matrice4td');
