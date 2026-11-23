-- Super Admin (peran sistem) memegang semua izin, termasuk izin yang ditambahkan migrasi berikutnya.
INSERT INTO role_permissions (role_id, permission_key)
SELECT r.id, p.key FROM roles r CROSS JOIN permissions p WHERE r.system AND r.key = 'super_admin'
ON CONFLICT DO NOTHING;

CREATE FUNCTION grant_new_permission_to_super_admin() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO role_permissions (role_id, permission_key)
    SELECT id, NEW.key FROM roles WHERE system AND key = 'super_admin'
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$;
CREATE TRIGGER permissions_grant_super_admin AFTER INSERT ON permissions
    FOR EACH ROW EXECUTE FUNCTION grant_new_permission_to_super_admin();
