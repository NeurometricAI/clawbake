ALTER TABLE instance_defaults
    ADD COLUMN gateway_config TEXT NOT NULL DEFAULT '{"gateway":{"controlUi":{"allowInsecureAuth":true,"dangerouslyDisableDeviceAuth":true,"dangerouslyAllowHostHeaderOriginFallback":true},"http":{"endpoints":{"chatCompletions":{"enabled":true}}}}}';
