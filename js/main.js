const { Kafka } = require('kafkajs');
const uuidv4 = require('uuid').v4;
const moment = require('moment');
const { Partitioners } = require('kafkajs')

const kafka = new Kafka({
  clientId: 'my-app',
  brokers: ['localhost:9092', 'localhost:9093'],
});

const producer = kafka.producer({ createPartitioner: Partitioners.LegacyPartitioner })

const uniqId = uuidv4();

producer.connect().then(() => {
  producer.send({
    topic: 'schedules',
    messages: [
      {
        headers: {
          "scheduler-epoch": `${moment().unix() + 10}`,
          "scheduler-target-topic": "timers",
          "scheduler-target-key": `${uuidv4()}`
        },
        key: `${uniqId}`,
        value: `This is a scheduled message with id ${uniqId}`
      }
    ],
  }).then(() => {
    console.log(`${uniqId} added`)
    producer.disconnect()
  })
});
