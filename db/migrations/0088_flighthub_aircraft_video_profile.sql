-- Declares model support; actual channel indices still come from FH discovery.
update device_types set capability_profile_json=capability_profile_json||'{"stream.video.read":{"enabled":true},"stream.video.control":{"enabled":true}}'::jsonb,updated_at=now()
where type_key in('dji.dock2','dji.matrice3d','dji.matrice3td');
