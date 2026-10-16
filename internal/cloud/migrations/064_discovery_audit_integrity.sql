CREATE TRIGGER cloud_discovery_policy_immutable BEFORE UPDATE OR DELETE ON cloud_discovery_policies FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE TRIGGER cloud_discovery_override_immutable BEFORE UPDATE OR DELETE ON cloud_discovery_overrides FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE TRIGGER cloud_watch_exclusion_immutable BEFORE UPDATE OR DELETE ON cloud_watch_exclusions FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE TRIGGER cloud_audience_rebuild_immutable BEFORE UPDATE OR DELETE ON cloud_audience_rebuilds FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
