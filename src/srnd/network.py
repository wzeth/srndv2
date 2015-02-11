#
# network.py
#
from . import config
from . import nntp
from . import storage
from . import util
import asyncio
import logging
import os
import queue
import struct
import time


from hashlib import sha1

class NNTPD:
    """
    nntp daemon
    """

    def __init__(self, daemon_conf, feed_config, store_config):
        """
        pass in valid config from config parser
        """
        self.log = logging.getLogger('nntpd')
        self.bindhost = daemon_conf['bind_host']
        self.bindport = daemon_conf['bind_port']
        self.name = daemon_conf['instance_name']
        self.instance_name = self.name
        # TODO: move to use as parameter
        self.feed_config = feed_config
        self.store = storage.FileSystemArticleStore(self, store_config)
        self.feeds = list()

    def add_article(self, article_id):
        self.log.debug('article added {}'.format(article_id))
        for feed in self.feeds:
            feed.add_article(article_id)
        
    def generate_id(self):
        now = int(time.time())
        id = sha1(os.urandom(8)).hexdigest()[:10]
        return '<{}.{}@{}>'.format(now, id, self.name)

    def start(self):
        """
        start daemon
        bind to address given via config
        """
        self.loop = asyncio.get_event_loop()
        coro = asyncio.start_server(self.on_ib_connection, self.bindhost, self.bindport, loop=self.loop)
        self.serv = self.loop.run_until_complete(coro)
        print('nntpd serving on {}'.format(self.serv.sockets[0].getsockname()))
        self.create_outfeeds()

    def create_outfeeds(self):
        feeds = dict()
        for key in self.feed_config:
            if key.startswith('feed-'):
                cfg = self.feed_config[key]
                host, port = util.parse_addr(key[5:])
                key = '{}:{}'.format(host,port)
                feeds[key] = dict()
                feeds[key]['settings'] = cfg
                feeds[key]['config'] = self.feed_config[key]
        for key in feeds:
            feed = Outfeed(key, self, feeds[key])
            asyncio.async(feed.run())
            self.feeds.append(feed)

    def on_ib_connection(self, r, w):
        """
        we got an inbound connection
        """
        self.log.info('inbound connection made')
        conn = nntp.Connection(self, r, w, True)
        asyncio.async(conn.run())
        #self.feeds(

    def end(self):
        """
        end daemon activity
        """
        self.serv.close()
        self.loop.run_until_complete(self.serv.wait_closed())
        


class Outfeed:

    def __init__(self, addr, daemon, conf):
        self.addr = util.parse_addr(addr)
        self.daemon = daemon
        self.settings = conf['settings']
        self.feed_config = conf['config']
        self.log = logging.getLogger('outfeed-%s-%s' % self.addr)
        self.feed = None
        


    def add_article(self, article_id):
        self.log.debug('add article: {}'.format(article_id))
        if self.feed:
            asyncio.async(self.feed.send_article(article_id))

    @asyncio.coroutine
    def proxy_connect(self, proxy_type):
        if proxy_type == 'socks5':
            phost = self.settings['proxy-host']
            pport = int(self.settings['proxy-port'])
            r, w = yield from asyncio.open_connection(phost, pport)
            # socks 5 handshake
            w.write(b'\x05\x01\x00')
            _ = yield from w.drain()
            data = yield from r.readexactly(2)
            self.log.debug('got handshake')
            # socks 5 request
            if data == b'\x05\x00':
                self.log.debug('handshake okay')
                req = b'\x05\x01\x00\x03' + self.addr[0].encode('ascii') + struct.pack('>H', self.addr[1])
                w.write(req)
                _ = yield from w.drain()
                self.log.debug('get response')
                data = yield from r.readexactly(2)
                success = data == b'\x05\x00'
                self.log.debug('got response success is {}'.format(success))
                _ = yield from r.readexactly(2)
                dlen = yield from r.readexactly(1)
                self.log.debug('read host')
                _ = yield from r.readexactly(dlen[0] + 2)
                if success:
                    self.log.info('connected')
                    return r, w
                else:
                    self.log.error('failed to connect to outfeed')
        elif proxy_type == 'None' or proxy_type is None:
            r ,w = yield from asyncio.open_connection(self.addr[0], self.addr[1])
            return r, w
        else:
            self.log.error('proxy type not supported: {}'.format(proxy_type))

    @asyncio.coroutine
    def connect(self):
        self.log.info('attempt connection')
        if 'proxy-type' in self.settings:
            ptype = self.settings['proxy-type']
            pair = yield from self.proxy_connect(ptype)
            if pair:
                return pair[0], pair[1]
        else:
            r, w = yield from asyncio.open_connection(self.addr[0], self.addr[1])
            return r, w    

    @asyncio.coroutine
    def run(self):
        self._run = True
        while self._run:
            if self.feed is None:
                pair = yield from self.connect()
                if pair:
                    r, w = pair
                    self.log.info('connected')
                    self.feed = nntp.Connection(self.daemon, r, w)
                    asyncio.async(self.feed.run())
            else:
                _ = yield from asyncio.sleep(1)
