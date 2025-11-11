// 	async createEmailTemplate=()=> await prisma.templateVersion.create({
//   data: {
//     template: {
//       create: {
//         name: 'Order Confirmation',
//         event: 'ORDER_CONFIRMATION',
//         channel: 'EMAIL',
//         lang: 'en',
//       },
//     },
//     version: 1,
//     subject: 'Your Order {{order.id}} is Confirmed!',
//     body: 'Hi {{user.name}}, Thanks for shopping! Your order {{order.id}} for ${{order.total}} has shipped to {{order.address}}. Track: {{order.tracking_url}}.',
//     variables: {
//       'user.name': 'string',
//       'order.id': 'string',
//       'order.total': 'number',
//       'order.address': 'string',
//       'order.tracking_url': 'string',
//     },
//   },
// });

// // Push variant (shorter)
// await prisma.templateVersion.create({
//   data: {
//     template: {
//       create: {
//         name: 'Order Shipped Push',
//         event: 'ORDER_SHIPPED',
//         channel: 'PUSH',
//         lang: 'en',
//       },
//     },
//     version: 1,
//     title: 'Order Shipped! 🚀',
//     body: 'Your {{order.id}} is on its way. Track: {{order.tracking_url}}',
//     variables: { 'order.id': 'string', 'order.tracking_url': 'string' },
//   },
// });
