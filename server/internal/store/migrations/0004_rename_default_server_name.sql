UPDATE server_identity SET name = 'Waveguide' || substr(name, length('OTA Viewer') + 1)
WHERE name = 'OTA Viewer' OR name LIKE 'OTA Viewer on %';
