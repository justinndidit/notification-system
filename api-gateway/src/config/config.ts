export default () => ({
  port: parseInt(process.env.PORT || '3000', 10),
  userServiceUrl: process.env.USER_SERVICE_URL,
  orchestratorUrl: process.env.ORCHESTRATOR_SERVICE_URL,
  templateServiceUrl: process.env.TEMPLATE_SERVICE_URL,
  emailServiceUrl: process.env.EMAIL_SERVICE_URL,
  pushServiceUrl: process.env.PUSH_SERVICE_URL,
  redisUrl: process.env.REDIS_URL,
  jwtSecret: process.env.JWT_SECRET,
});
